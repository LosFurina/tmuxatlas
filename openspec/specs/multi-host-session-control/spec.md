# multi-host-session-control Specification

## Purpose

定义跨 Hub 与 Agent 的显式 session target、统一操作路由、确认结果、幂等执行和终端输入隔离。

## Requirements

### Requirement: Atomic multi-host session target
TmuxAtlas SHALL 将 `host_id` 与 `session` 表示为不可分割的 session target，并在 API
入口、Router、Peer request、PTY stream 和 tmux executor 之间原样传递该 target。
TmuxAtlas MUST 拒绝不完整、未知或已经失效的 target，且不得把远程 target 回退为 Hub
本机 session。

#### Scenario: Route an explicit local target
- **WHEN** 请求携带 Hub 本机 `host_id` 与一个有效 session
- **THEN** Router 在 Hub 本机执行操作，并保持该 host+session target 不变

#### Scenario: Route an explicit remote target
- **WHEN** 请求携带已连接 Agent 的 `host_id` 与属于该 Agent 的有效 session
- **THEN** Router 只把请求发送给该 Agent 的当前 connection generation

#### Scenario: Reject an incomplete remote target
- **WHEN** 请求缺少 `host_id`、缺少 session 或引用不属于目标 host 的 session
- **THEN** TmuxAtlas 返回结构化 `invalid-target` 或 `not-found`，且不执行任何本机 tmux 命令

#### Scenario: Target a session to be created
- **WHEN** `new` action 携带目标 host 和待创建 session 名称
- **THEN** 该名称作为同一原子 target 路由到目标 host，并只在该 host 上创建

### Requirement: Unified local and remote action routing
TmuxAtlas SHALL 通过同一 Router/Executor contract 执行 local 与 remote 的 rename、
new、select-window、select-pane 及其他 session action。每个 action MUST 在执行前验证
目标 host、session、当前 connection generation 和所需 capability。

#### Scenario: Rename a remote session
- **WHEN** 调用方请求重命名一个 remote host 上的 session
- **THEN** Hub 把包含原 target 和新名称的 correlated request 发送给该 host，且不重命名 Hub 本机的同名 session

#### Scenario: Select a remote window and pane
- **WHEN** 调用方选择 remote target 的 window，并可选指定 pane
- **THEN** 选择操作只在拥有该 target 的 Agent 上执行

#### Scenario: Peer becomes offline before routing
- **WHEN** Router 解析 remote target 后发现目标 Peer 没有 active generation
- **THEN** 请求以 `peer-offline` 失败，且不得进入旧连接发送队列

#### Scenario: Generation changes before execution
- **WHEN** action 绑定的 connection generation 在 Agent 执行前已被替换
- **THEN** 旧 generation 拒绝该 action，Hub 返回明确失败而不把请求转移到新 generation

### Requirement: Confirmed action outcomes
TmuxAtlas SHALL 为每个有结果的 session action 分配唯一 `request_id`，并 MUST 在目标
Executor 返回 terminal result 后才报告成功。入队、发送或 accepted ack 均不得单独
视为执行成功。

#### Scenario: Remote action succeeds
- **WHEN** Agent 成功完成 request 对应的 tmux 操作并返回 correlated result
- **THEN** Hub 以该 result 完成原调用，且成功响应引用相同 `request_id`

#### Scenario: Remote tmux execution fails
- **WHEN** Agent 的 tmux executor 返回 not-found 或 execution error
- **THEN** Agent 返回 correlated structured error，Hub 将该失败映射到原调用而不返回成功

#### Scenario: Action times out after acceptance
- **WHEN** Agent 已返回 accepted ack 但未在 deadline 前产生 terminal outcome
- **THEN** Hub 以 `timeout` 完成原调用，并忽略该 request 的任何 late terminal response

#### Scenario: Send queue is full
- **WHEN** request 无法进入目标 Peer 的有界发送队列
- **THEN** Hub 立即返回 `queue-full`，且不得生成虚假 accepted ack

### Requirement: Idempotent state-changing requests
TmuxAtlas MUST 以 `request_id` 和规范化 payload digest 去重可重试的 rename、new、select
及其他有副作用 action。同一 Agent process instance 内，相同 ID 与相同 payload SHALL
至多执行一次；相同 ID 与不同 payload MUST 被拒绝。

#### Scenario: Retry a completed request
- **WHEN** Agent 再次收到具有相同 `request_id` 和相同 payload digest 的已完成 request
- **THEN** Agent 返回缓存的同一 terminal outcome，且不再次执行 tmux 操作

#### Scenario: Duplicate an in-flight request
- **WHEN** Agent 在首次 request 尚未完成时收到相同 ID 与 payload 的重试
- **THEN** 重试关联到首次执行并取得同一 terminal outcome

#### Scenario: Reuse a request ID with different payload
- **WHEN** Agent 收到已存在 `request_id` 但 payload digest 不同的 request
- **THEN** Agent 返回 `request-conflict`，且不执行新 payload

#### Scenario: Agent instance changes after ambiguous execution
- **WHEN** request 的结果在断线时未知且重连后的 Agent instance ID 已改变
- **THEN** Hub 返回 `execution-unknown`，且不得自动重放该副作用操作

### Requirement: Host-bound terminal input routing
TmuxAtlas SHALL 把每个 terminal input stream 绑定到一个固定 session target 和
connection generation。Input frame MUST 在 stream 内有序并可识别重复；断线后的新
stream MUST NOT 静默重放旧 stream 中交付状态未知的 input。

#### Scenario: Deliver input to a remote session
- **WHEN** 浏览器向绑定 remote host+session target 的 active PTY stream 发送 input
- **THEN** Hub 按 sequence 将 input 交付给该 target 所属 Agent 的 PTY，且不写入 Hub 本机 PTY

#### Scenario: Receive duplicate input sequence
- **WHEN** Agent 在同一 stream 收到已经确认的 input sequence
- **THEN** Agent 忽略重复 frame，且终端只接收一次对应按键

#### Scenario: Receive an input sequence gap
- **WHEN** Agent 收到高于下一预期 sequence 的 input frame
- **THEN** Agent 以明确 stream protocol error 关闭或拒绝该 stream，而不乱序写入 PTY

#### Scenario: Reconnect after uncertain input delivery
- **WHEN** PTY stream 在 input 是否已执行未知时断开并建立新 stream
- **THEN** TmuxAtlas 不把旧 input 自动发送到新 stream，并向上游暴露旧 stream 已关闭

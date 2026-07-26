# peer-runtime-protocol Specification

## Purpose

定义身份认证后的 Peer runtime 协商、generation、实时撤销、correlated request 与远程 PTY 生命周期。

## Requirements

### Requirement: Authenticated runtime activation
TmuxAtlas SHALL 把现有 `peer-transport` 完成的 transport 与 Ed25519 identity 认证作为
Peer runtime protocol 的前置条件。Peer MUST 在身份认证后完成 runtime protocol
协商，才能注册为 active、同步状态、接收 action 或建立 PTY。

#### Scenario: Authenticated Peer enters negotiation
- **WHEN** Peer 已按 `peer-transport` 通过 Ed25519 challenge authentication
- **THEN** Hub 启动 runtime hello 协商，但在协商成功前不把该连接注册为 active

#### Scenario: Runtime message arrives before activation
- **WHEN** 未完成协议协商的连接发送 state、action 或 PTY runtime message
- **THEN** Hub 拒绝该消息并关闭或终止未激活的 runtime connection

#### Scenario: Headless Agent becomes active
- **WHEN** outbound-only Agent 完成身份认证并协商兼容 runtime protocol
- **THEN** Agent 才开始状态同步并接受协商 capability 覆盖的 action 与 PTY request

### Requirement: Explicit Peer protocol negotiation
TmuxAtlas SHALL exchange supported protocol versions, capabilities, product build version and Agent
instance identity after Ed25519 authentication and before the Peer becomes active. Both sides MUST use
one mutually supported protocol version and MUST gate optional behavior on negotiated capabilities.

#### Scenario: Negotiate a common protocol
- **WHEN** Hub 与 Agent 声明的 protocol version 范围存在交集
- **THEN** 双方选择一个共同版本、记录共同 capabilities，并仅在此后把连接标记 active

#### Scenario: No common protocol version
- **WHEN** Hub 与 Agent 没有共同支持的核心 protocol version
- **THEN** Hub 返回结构化 `protocol-incompatible` 并关闭连接，不处理状态、action 或 PTY 消息

#### Scenario: Missing optional capability
- **WHEN** 核心版本兼容但 Agent 缺少某个 request 所需 capability
- **THEN** 该 request 以 `capability-unsupported` 失败，且不得被 best-effort 执行

#### Scenario: Legacy peer omits hello
- **WHEN** 已认证 Peer 未在握手 deadline 内发送所需 hello
- **THEN** Hub 以明确 incompatibility reason 关闭连接，而不推断旧 payload 兼容

### Requirement: Generation-scoped Peer connection
TmuxAtlas SHALL 为同一 Peer identity 的每次 active control connection 分配单调递增的
connection generation。新 generation MUST 原子替换并取消旧 generation；旧 handler
MUST NOT 修改、关闭或把较新的 generation 标记为 offline。

#### Scenario: Replace an active connection
- **WHEN** 同一 Peer identity 的新连接完成认证和协议协商
- **THEN** Hub 原子安装更大的 generation，并取消旧 generation 的 sender、requests 和 PTY resources

#### Scenario: Old handler exits after replacement
- **WHEN** 被替换的旧 handler 在新 generation active 后延迟退出
- **THEN** cleanup 只移除旧 generation 的资源，且 host 保持由新 generation 标记为 online

#### Scenario: Send races with replacement
- **WHEN** Router 发送 request 的同时目标 Peer 被新 generation 替换
- **THEN** Send 操作要么由绑定 generation 接受，要么返回明确 stale/offline error，且不得向已关闭 channel 发送

### Requirement: Runtime Peer revocation
TmuxAtlas SHALL 让运行中的 Hub 原子提交 Peer identity 撤销。成功撤销 MUST 同时更新
持久化授权与内存授权，并立即取消该 identity 的 active control connection、未完成
request、pending PTY 和 active PTY；后续握手 MUST 被拒绝。

#### Scenario: Revoke a connected Peer
- **WHEN** operator 成功撤销一个当前在线的 Peer identity
- **THEN** Hub 持久化撤销、从运行时授权表移除 identity，并立即断开该 Peer 的全部 generation-scoped resources

#### Scenario: Revocation persistence fails
- **WHEN** Hub 无法原子写入撤销后的 Peer store
- **THEN** 撤销操作返回失败，且不得只在内存中产生无法持久化的部分撤销

#### Scenario: Revoked Peer reconnects
- **WHEN** 已撤销 identity 再次完成 transport connection 并提交签名
- **THEN** Hub 在当前运行周期内拒绝握手，且不依赖重启重新加载 Peer store

#### Scenario: Revoke while requests are pending
- **WHEN** Peer 被撤销时仍有 correlated requests 等待 terminal outcome
- **THEN** 所有 pending requests 只完成一次 `peer-revoked` error，且 late responses 被忽略

### Requirement: Correlated Peer request outcomes
TmuxAtlas SHALL 为有结果的 Peer control operation 使用包含 `request_id`、target、
connection generation 和 deadline 的 request envelope。Agent MAY 返回非终态 accepted
ack，但 MUST 为每个 accepted request 返回且只返回一个 correlated result 或 structured
error。

#### Scenario: Accept and complete a request
- **WHEN** Agent 接受 request 并成功完成目标操作
- **THEN** Agent 先返回可选 accepted ack，再返回引用相同 `request_id` 的唯一 terminal result

#### Scenario: Reject a request before execution
- **WHEN** request 的 generation、target、capability 或 deadline 无效
- **THEN** Agent 返回 correlated structured error，且不执行 payload

#### Scenario: Connection closes with pending requests
- **WHEN** control connection 在 requests 等待 terminal outcome 时关闭
- **THEN** Hub 以明确 connection failure 完成该 generation 的每个 pending request，且不静默丢弃

#### Scenario: Late response from an old generation
- **WHEN** Hub 收到 request ID 匹配但 connection generation 已过期的 result
- **THEN** Hub 忽略该 result，且不得用它完成新 generation 的 request

### Requirement: Managed remote PTY lifecycle
TmuxAtlas SHALL 把 remote PTY 绑定到唯一 Peer identity、connection generation、
session target 和一次性 attach token。PTY 任一参与方结束时 MUST 幂等 teardown 两端
WebSocket、relay goroutines、PTY fd 和 tmux 子进程。Remote resize MUST 到达并更新
目标 Agent 上的 PTY。

#### Scenario: Establish a remote PTY
- **WHEN** Agent 为有效 target 创建 PTY，并用匹配 identity、generation 与一次性 token 连接 data channel
- **THEN** Hub 绑定 browser 与 Agent stream，并在绑定成功后完成 PTY open result

#### Scenario: Reject a late PTY connection
- **WHEN** Agent data channel 在 pending stream 超时、Peer 被替换或撤销后才到达
- **THEN** Hub 拒绝 late connection，且不得复活已结束 stream

#### Scenario: Browser disconnects while PTY is idle
- **WHEN** browser WebSocket 在 Agent PTY read 阻塞且没有输出时关闭
- **THEN** teardown 先关闭 PTY 与连接以解除阻塞，再等待所有 relay goroutine 和子进程退出

#### Scenario: Agent or control connection disconnects
- **WHEN** PTY 所属 Agent data channel 或 control generation 结束
- **THEN** Hub 关闭对应 browser stream，并只回收该 generation 所拥有的 PTY resources

#### Scenario: Resize a remote PTY
- **WHEN** browser 为 active remote stream 发送有效的正数 cols/rows resize
- **THEN** Hub 将 typed resize frame 路由到所属 Agent，Agent 对目标 PTY 应用该尺寸

#### Scenario: Repeat teardown
- **WHEN** browser、Hub timeout 与 Agent 几乎同时请求关闭同一 PTY stream
- **THEN** teardown 只执行一次，所有参与方最终退出，且不会 double-close panic 或遗留 tmux attach 子进程

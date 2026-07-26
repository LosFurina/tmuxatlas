## Context

TmuxAtlas 的 Hub 通过 `/ws/peer` 与每个 Agent 保持控制 WebSocket，并通过
`/ws/peer-pty` 为远程终端建立独立数据通道。当前实现把 Peer 连接保存在按 host
fingerprint 索引的内存表中，调用方直接取得连接及发送队列；同一身份重连时，新旧
handler 没有代次隔离。旧 handler 的退出、发送队列关闭或撤销操作因此可能影响已经
替换它的新连接。

多主机 session 操作目前也没有统一的领域模型。不同 HTTP handler 分别解释 `host`、
session name 和 action payload，部分路径在缺少 host 时直接执行 Hub 本机 tmux 命令。
远程 action 在写入 Peer 队列后即向调用方返回成功，Agent 的最终 tmux 错误不会回到
原请求。PTY open、terminal input 与 resize 又走另一套无结果关联的 stream 流程，
无法共享目标校验和失败语义。

本设计涉及 Hub、Agent、Peer wire protocol、HTTP action handler、PTY relay、tmux
子进程和 Peer 持久化，是跨模块的一致性变更。设计必须同时满足以下约束：

本 change 新建 `peer-runtime-protocol` capability，并建立在现有 `peer-transport`
已经规定的系统信任、Ed25519 identity、网关路由和 outbound Agent 约束之上。
运行时规范只定义认证完成后的协商、连接代次、请求与 PTY 生命周期，不修改或重写
`peer-transport` 基线，以避免两个 active change 在归档时覆盖同一 capability。

- Hub 与 Agent 可以因发布和重启而独立断线，但不允许把不兼容消息解释为成功。
- WebSocket 只保证单条存活连接内有序，不保证跨连接重放安全。
- tmux 命令不是事务；Agent 可能在执行成功后、返回结果前崩溃。
- 任意网络写入、PTY read 或子进程 Wait 都可能阻塞，不能在持有全局 registry lock
  时执行。
- 同一 Peer identity 在 Hub 上任意时刻只能有一个 active connection generation。

## Goals / Non-Goals

**Goals:**

- 让 host 与 session 作为原子目标贯穿 API、Peer request、PTY 和 tmux execution。
- 让 local 与 remote rename、new、select、input 和 resize 使用同一套路由及结果模型。
- 在身份认证后协商明确的 protocol version 与 capabilities，并对不兼容情况确定失败。
- 用 connection generation 隔离快速重连中的新旧 handler、发送队列、pending request
  和 PTY stream。
- 让 Peer 撤销同时更新持久化与运行时授权状态，并立即断开对应资源。
- 为有结果的操作提供 request correlation、accepted acknowledgement、唯一 terminal
  result/error 和有界幂等去重。
- 让 PTY 任一侧退出都能幂等、对称地回收 goroutine、WebSocket、PTY fd 和 tmux
  子进程，并让远程 resize 真正作用到目标 Agent。

**Non-Goals:**

- 不重新设计 pairing code、TLS termination、Ed25519 密钥格式或公网接入策略。
- 不修改现有 `peer-transport` capability 的信任、网关或 Agent 服务 requirements。
- 不增加 Web 状态页、离线体验或新的终端断线提示交互。
- 不承诺跨 Agent 进程崩溃的 exactly-once tmux 副作用；无法证明执行结果时必须返回
  `execution-unknown`，而不是自动重放。
- 不在断线后恢复旧 PTY 或重放 terminal input；恢复必须建立新的 stream。
- 不改变 tmux 自身的 session/window/pane 语义。

## Decisions

### 1. 使用规范化的原子 SessionTarget 和统一 Executor

所有 session 相关入口在边界处构造不可变的：

```text
SessionTarget {
  host_id
  session
}
```

`host_id` 与 `session` 必须同时存在。`new` action 中的 `session` 表示待创建的名称；
其余 action 表示必须已经存在的目标。rename 的新名称属于 action payload，执行成功
后结果返回新的 `SessionTarget`。

Hub 只在请求入口解析一次 target，然后交给统一 Router。Router 根据 `host_id` 选择
`LocalExecutor` 或绑定当前 connection generation 的 `PeerExecutor`。后续层不得因
Peer 不存在、字段为空或 target 过期而回退到 LocalExecutor。PTY open 同样先经过
Router，并把解析后的 target 固定在 stream 对象上。

采用统一 Executor 而不是继续在每个 HTTP handler 内增加 host 分支，是为了让本机与
远程操作共享验证、request ID、deadline、结果和错误映射，也避免未来新增 action 时
再次漏掉 host 路由。

### 2. 身份认证与协议协商是两个连续但独立的阶段

控制连接按以下状态机建立：

```text
transport connected
  -> Ed25519 challenge authenticated
  -> hello / hello-ack negotiated
  -> active
  -> draining
  -> closed
```

`hello` 携带支持的 protocol version 范围、capability 集合、build version 和本次
connection generation；`hello-ack` 选择唯一 protocol version 并确认双方共同
capabilities。核心版本没有交集时，Hub 返回结构化 `protocol-incompatible` 后关闭
连接。核心版本兼容但某个可选 capability 缺失时，连接仍可服务共同能力，对相关请求
返回 `capability-unsupported`。

连接在协商完成前不得注册为 active，不得同步状态、接受 action 或建立 PTY。旧版
Agent 未发送 hello 时，Hub 在固定握手 deadline 后以明确 incompatibility close
结束连接，不尝试猜测 payload 版本。

版本与 capabilities 分离，可以在不提升核心协议版本的情况下独立演进
`session-actions`、`request-results` 和 `pty-control`；相比仅比较产品版本字符串，
这能表达 backport 和渐进实现的真实兼容性。

### 3. 有结果操作使用统一 request lifecycle

Peer request envelope 包含：

```text
request_id
connection_generation
deadline
operation
target
payload
```

Agent 可以先返回非终态 `ack(accepted)`，但每个 request 最终必须且只能产生一个
`result` 或结构化 `error`。Hub 只有收到 terminal result 后才向上游报告成功；
send queue full、断连或 capability 缺失在 Hub 本地直接完成为 error，不能伪造 ack。

Hub 为每个 generation 维护 pending request registry。响应必须同时匹配 request ID
和 generation；late response、重复 terminal response 和旧 generation response
只记录诊断信息，不得完成新请求。连接关闭时，该 generation 的所有 pending request
立即以明确错误结束。

错误使用稳定 code，至少区分 `invalid-target`、`not-found`、`peer-offline`、
`peer-revoked`、`protocol-incompatible`、`capability-unsupported`、`queue-full`、
`timeout`、`execution-failed`、`execution-unknown` 和 `request-conflict`。HTTP 层
集中映射这些 code，不直接暴露任意 tmux stderr。

相比 fire-and-forget 消息，这一生命周期会增加一次或两次控制消息和一份 pending
状态，但它是调用方区分“已入队”与“已执行”的必要成本。

### 4. 幂等以 request ID、payload digest 和 Agent instance 为边界

Agent 维护有界的 request outcome cache。首次收到 request ID 时记录 payload digest
和 in-flight 状态；相同 ID、相同 digest 的并发或重试请求加入同一执行并取得同一个
terminal outcome；相同 ID、不同 digest 返回 `request-conflict`。默认 cache TTL 为
5 分钟、最多 1024 个 outcome、单个序列化 result/error 最大 64 KiB；三项均可配置，
并以较小者优先受全局内存上限约束。完成结果在此范围内缓存，以覆盖网络超时后的正常
重试；容量耗尽且无法淘汰已完成项时，新 request 明确返回 `resource-exhausted`。

去重只承诺在同一 Agent process instance 内有效。Agent hello 携带新的 instance ID；
若连接断开后 instance ID 已改变，Hub 不自动重放结果未知的副作用操作，而是完成为
`execution-unknown`。这避免用无法持久化证明的机制声称跨崩溃 exactly-once。

Terminal input 不进入普通 action outcome cache。Hub 在单个 PTY stream 内为 input
分配单调 sequence，Agent 只接受预期 sequence、忽略已经确认的重复 sequence，并拒绝
缺口或旧 stream 的 frame。stream 断开后不跨新 stream 重放未确认 input。

### 5. Peer registry 以 generation 比较交换

Peer registry 的 active entry 包含 identity、generation、连接 context、私有 sender、
pending request 集合和所属 PTY 集合。新连接完成认证及协商后，Hub 在 registry lock
内分配更大的 generation、安装新 entry，并取得旧 entry；释放 lock 后再取消旧 entry
并等待其 goroutine 退出。

每个 handler 的 cleanup 必须携带自己的 entry/generation，并通过 compare-and-swap
确认它仍是当前 entry 后才能把 host 标记 offline。旧 handler 始终可以清理自己的
资源，但不能关闭、清空或改写新 generation。

发送队列封装在 `PeerConnection.Send(ctx, message) error` 内，不向 Router 或 HTTP
handler 暴露 channel，也不允许外部 close。单一 writer goroutine 拥有 WebSocket
data writes；取消 context、关闭 WebSocket 和等待 writer 的顺序由 connection entry
统一管理。这样消除 send-on-closed-channel，并把 queue full 变成普通错误。

### 6. 撤销通过正在运行的 Hub 单一提交

运行中的 Hub 是 Peer 授权状态的单一写入者。`peers remove` 在 Hub 运行时通过用户私有
Unix socket 调用 runtime revoke，而不是由另一个进程直接修改 `peers.json`。Hub 在
Peer registry/store lock 下先原子写入新的 Peer store；持久化失败则不改变运行时授权。
持久化成功后从授权表移除 identity，再取消 active generation、pending request 和
PTY stream，并向连接发送 best-effort `peer-revoked` close。

Hub 未运行时，CLI 可以直接原子更新 Peer store；下次启动从已撤销状态加载。任何新
握手都从当前内存授权表检查 identity，因此一次成功的在线撤销不依赖重启生效。

选择 Unix socket 命令而不是文件 watch，是为了让“持久化、内存状态和资源取消”保持
一个提交顺序，并避免文件替换事件、半写文件和 watcher 延迟造成授权窗口。

### 7. PTY stream 是 generation-scoped 的拥有型资源

每个 PTY stream 对象拥有 target、host identity、connection generation、一次性 attach
token、context、两端 WebSocket、PTY fd、tmux 子进程和所有 relay goroutine。Agent
只能用控制连接下发的一次性 token 完成对应 pending stream；host、generation、token
或 target 任一不匹配的 late connection 均被拒绝。

Protocol v1 的 Peer PTY data channel 明确定稿为“binary data frame + JSON control
frame”，每个 WebSocket message 恰好承载一个 frame。`input`/`output` 使用 binary
message：固定 header 包含 magic、frame version、data direction 和 uint64 sequence，
payload 原样保留 terminal bytes。`resize`、`close`、`error` 使用 UTF-8 JSON control
message，至少包含 `version`、`type` 和 `sequence`；`resize` 还必须包含正数
`cols`/`rows`，`close`/`error` 的 reason 长度受限。Hub 把浏览器现有的 binary input
与 text resize 分别转换为 v1 binary data frame 和 JSON control frame；Agent 必须验证
header、JSON schema、sequence 与尺寸边界，再对 resize 调用目标 PTY 的 resize。Agent
的 PTY output 只允许回到同一 stream 所绑定的浏览器连接。该组合让 terminal data
避免 base64 膨胀，同时让低频控制消息保持可扩展、可诊断的结构化表示。

浏览器、Hub bridge、Peer data WebSocket、Peer control generation、PTY read/write 或
tmux 子进程任一侧结束时，统一调用 `Teardown(reason)`。Teardown 使用 `sync.Once`：
先取消 context 并关闭 WebSocket 与 PTY fd，使阻塞 I/O 返回，再等待 relay goroutine；
tmux 子进程若未在宽限期退出，则终止并最终强制回收。不得先等待一个仍依赖 deferred
close 才能退出的 goroutine。

PTY open 只有在 Agent 创建 PTY 且 data channel 绑定成功后才返回 terminal success。
建立失败、15 秒等待超时、Peer 替换或撤销都返回可关联 error，并关闭浏览器侧 stream。

### 8. 并发所有权和测试围绕状态机，而不是共享 channel

连接、request 和 PTY 各自由单一生命周期对象拥有 context 与 WaitGroup。全局 lock
只保护 registry/map 的短临界区；网络 I/O、tmux execution 和 goroutine join 均在锁外。
所有 close 都由 owner 的幂等方法完成。

测试使用可控 fake connection、fake tmux executor 和同步 barrier 复现以下交错：

- 新 generation active 后旧 handler 延迟退出。
- Router 取得连接同时发生 replace/revoke。
- writer 被背压时 connection teardown。
- request ack 后断连、result 在 timeout 后到达以及重复 request。
- 浏览器在 idle PTY read 阻塞时断开。
- PTY attach 在 timeout、replace 或 revoke 后迟到。
- resize 与 input 到达远端目标而不落到 Hub 本机。

关键并发测试必须在 `go test -race` 下运行。

## Risks / Trade-offs

- [协议握手是 breaking change，旧 Agent 会暂时离线] → 发布说明要求先更新 Hub、再更新
  Agent；Hub 对缺少 hello 的连接返回明确版本错误，并在 hosts/CLI 诊断数据中保留最后
  上报的产品版本，便于定位待升级节点。
- [等待 Agent terminal result 会提高 HTTP action 延迟] → ack 与 terminal result 分离，
  使用每类操作的有界 deadline；调用方得到 timeout 而不是虚假成功。
- [内存 outcome cache 无法覆盖 Agent 崩溃] → hello 暴露 Agent instance ID；跨 instance
  的未知请求不自动重放，返回 `execution-unknown`。
- [新连接替换旧连接会中断旧 generation 的活跃 PTY] → PTY 明确绑定 generation 并快速
  teardown；客户端需要建立新 stream，不尝试迁移交互式终端输入。
- [撤销需要经运行中 Hub，CLI 行为比直接改文件复杂] → CLI 自动优先使用 Unix socket，
  Hub 不运行时保留离线原子修改路径，并为两条路径提供一致输出。
- [request cache 与 pending registry 增加内存占用] → 对数量、结果大小和 TTL 设置硬上限，
  达到上限时用明确 `queue-full`/`resource-exhausted` 失败。
- [typed PTY frame 改变 Agent 数据通道格式] → 通过 `pty-control` capability 协商；不支持
  该 capability 的 Peer 不允许建立需要 resize/input 保证的新 PTY。

## Migration Plan

1. 先在共享 protocol package 中定义 hello、capabilities、generation、request outcome、
   error code 和 typed PTY frame，并为编码/解码与状态机增加契约测试。
2. 更新 Hub，使未协商连接不能进入 active registry；保留现有持久化 Peer identity，
   不要求重新 pairing。
3. 更新 Agent 完成 hello、request outcome、去重 cache 和新 PTY frame；部署顺序为 Hub
   先、Agent 后。旧 Agent 在过渡期间会收到明确 incompatibility 并保持重连退避。
4. 将 HTTP local/remote action 迁移到统一 Router/Executor。所有新的 mutating action
   API 强制显式传 `host_id` 与 session；嵌入前端在同一版本同步升级，不保留省略 host
   并默认本机的兼容路径。删除 handler 内缺少 host 时的回退逻辑。
5. 将 `peers remove` 迁移为在线 Unix socket revoke，并验证现有连接、request 与 PTY
   会立即取消。
6. 启用 generation-scoped registry 和拥有型 PTY teardown，运行 race、断连、超时和
   fault-injection 测试后发布。

回滚必须同时考虑协议兼容：若 Hub 回滚到不支持新握手的版本，已经升级的 Agent 也必须
回滚到对应协议版本，或 Hub 必须保留双方都明确实现的旧版本。Peer identity/store 格式
不改变，因此二进制回滚不需要重新 pairing；回滚期间不得把不兼容 action 当作成功。

## Open Questions

None. Protocol v1 已定稿为带固定 header 的 binary data frame 加 JSON control frame；
outcome cache 默认 TTL 为 5 分钟、每 Peer 最多 1024 个 outcome、单个序列化结果最多
64 KiB，且三项均可配置；所有新的 mutation API 必须显式携带 `host_id` 与 session。
实现只能在不改变对应 specs 行为的前提下调整内部结构。

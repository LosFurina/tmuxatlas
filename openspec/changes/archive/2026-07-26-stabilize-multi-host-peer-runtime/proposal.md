## Why

TmuxAtlas 已能通过 Hub 聚合多台 Agent 的 tmux 会话，但当前运行时仍把“目标定位、消息入队、命令执行和连接存活”当作彼此松散的步骤。Host 与 session 没有作为一个不可分割的目标贯穿请求链路，部分 rename、terminal input 和 session/window action 因此可能落到 Hub 本机、错误主机或已经失效的 Peer 连接；Hub 在消息进入发送队列后也可能提前报告成功，而 Agent 上的 tmux 操作稍后失败且只留下日志。

Peer 生命周期同样缺少明确的一致性边界：同一身份快速重连时，旧 handler 的退出清理可能覆盖新连接；撤销已配对身份不会立即终止现有连接；控制消息没有 protocol version、capabilities、request ID 或结构化结果，导致 Hub 与 Agent 独立升级时只能静默假设兼容。PTY relay 还缺少对称 teardown，远程 resize 没有真正到达 Agent，断连、超时和 late connection 容易留下资源或产生无原因的关闭。

本 change 将把 multi-host runtime 收敛为一套可关联、可拒绝、可重试且不会误路由的协议，使每个跨主机操作要么在唯一目标上得到明确结果，要么以可识别错误失败。

## What Changes

- 将 `host + session` 定义为单一、不可分割的 session target，并在 HTTP/API 请求、Peer control message、PTY open、terminal input 和 tmux execution 全链路携带；请求进入 Hub 后只解析一次目标，不允许远程目标在后续步骤回退为本机默认值。
- 统一 local 与 remote session control 的路由规则，使 rename、terminal input、new/select 等 session/window action 都先验证目标主机、连接代次与目标 session，再交给目标主机执行；返回成功必须代表目标 Agent 已完成对应 tmux 操作，而不是仅代表消息已进入 Hub 发送队列。
- 为 Peer control protocol 增加显式 `protocol_version`、`capabilities` 和兼容性协商。Hub 与 Agent 仅使用双方共同支持的消息与行为；无共同版本或缺少必需 capability 时，以结构化 incompatibility error 拒绝相关连接或请求。
- **BREAKING (Peer protocol)** 要求建立认证后的 Peer control channel 在参与状态同步或执行控制操作前完成协议协商。旧版 Agent 若没有受支持的兼容路径，将被明确拒绝并需要与 Hub 协调升级，而不再被静默当作兼容节点。
- 为有结果的控制操作引入统一 request envelope，至少包含 `request_id`、目标、操作类型、payload、协议/能力上下文和 deadline；Agent 返回 correlated `ack`、成功结果或结构化 `error`，Hub 将 offline、revoked、incompatible、not-found、timeout 和 execution-failed 等失败原样映射给调用方。
- 为可重试的状态变更定义基于 `request_id` 的去重与幂等规则：相同请求的重试不得重复执行 rename/new/select 等副作用，并应返回首次执行的确定结果。Terminal input 使用有序 stream/sequence 语义，不在断线重连后静默重放可能重复的按键；无法确认交付时明确关闭或报错。
- 为每次成功认证的 Peer 连接分配单调递增的 connection generation。新连接原子替换同一身份的旧连接并取消其所有后台任务；旧连接只能清理属于自身 generation 的注册、发送队列和 PTY，不得将较新的连接标记 offline 或关闭。
- 将 Peer revoke 变成 Hub 运行时原子操作：持久化删除身份的同时更新内存授权状态，立即取消该身份的当前 control connection、pending/active PTY 和未完成请求；此后新的握手必须被拒绝。撤销完成后不得依赖 Hub 重启才生效。
- 重构 PTY 生命周期为双向、幂等 teardown：浏览器、Hub、Agent、tmux PTY 或 control connection 任一侧退出时，都取消关联 pump、关闭两端连接、释放 PTY 子进程并完成 pending request；重复 close、late PTY connection 和超时回调不得复活或重复释放 stream。
- 让远程 resize 与 terminal input 沿已绑定的 PTY stream 到达正确 host+session target。Resize 必须实际更新 Agent 上的 PTY；已过期、已撤销、generation 不匹配或目标不一致的 stream frame 必须被拒绝，而不是被忽略或转发到替代目标。
- 对 send queue full、Peer 断连、操作超时、Agent tmux 错误和 PTY 建立失败采用明确失败语义，并确保所有 correlated request 最终只得到一次 terminal outcome；禁止静默 drop 后仍向上游返回成功。
- 增加覆盖快速重复重连、旧连接延迟退出、运行时撤销、队列背压、请求重试去重、错误回传、浏览器/Peer 双向断连、PTY late arrival、远程 resize 和跨主机 action 路由的单元、集成及 race/fault-injection 测试。

## Capabilities

### New Capabilities

- `multi-host-session-control`: 定义 host+session 原子目标、local/remote 统一路由、rename/input/session/window action 的执行确认、request correlation、幂等重试、有序 terminal input 以及明确失败语义。
- `peer-runtime-protocol`: 定义建立在既有 peer transport 身份与网关约束之上的运行时协议，包括版本和 capability 协商、connection generation、实时撤销、correlated ack/result/error，以及 PTY teardown 与 resize 交付保证。

### Modified Capabilities

None. 运行时协议作为独立 capability 建立在现有 `peer-transport` 之上，不修改该基线。

## Impact

- Peer wire protocol、认证后握手顺序和 rolling-upgrade 兼容策略；Hub 与 Agent 可能需要协调升级，版本不兼容将从隐式异常变为显式拒绝。
- `pkg/peer` 中的 protocol envelope、Client、Handler、Manager、connection registry、send queue、request tracker、revocation path、PTY manager 和 PTY relay。
- `pkg/server` 中 multi-host session/action API 的 target schema、remote routing、超时等待和 HTTP error mapping；现有“入队即 204”行为将改为基于 Agent terminal outcome 的确定结果。
- `pkg/ws` 与 `pkg/tmux` 中 terminal stream pump、resize、PTY close/Wait 和子进程回收逻辑。
- `pkg/identity` 及 `peers remove` 的运行时一致性：撤销操作必须作用于正在运行的 Hub 授权状态，并同步关闭现有资源，而不只修改磁盘文件。
- 现有 Peer/PTY 测试需要升级为带 protocol negotiation、request correlation、generation 和 structured error 的契约测试，并增加并发、断连、超时、背压及幂等故障注入覆盖。
- 本 change 不改变既有公网部署拓扑、TLS/Ed25519 信任模型或已保存的 tmux session 数据；主要兼容性风险集中在 Hub-Agent wire protocol 与 action API 的成功/失败语义。

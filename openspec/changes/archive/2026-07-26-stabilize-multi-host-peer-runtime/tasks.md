## 1. 运行时协议类型与协商

- [x] 1.1 在 `pkg/peer` 定义并验证 hello/hello-ack 类型，覆盖 protocol version 范围、capabilities、build version、Agent instance ID 和序列化往返测试。
- [x] 1.2 定义 request、accepted ack、terminal result 与 structured error envelope，强制校验 `request_id`、connection generation、deadline、operation、target 和 payload。
- [x] 1.3 在现有 Ed25519 认证之后实现 hello 协商状态机与握手 deadline，只有选择共同版本和 capabilities 后才能注册 active Peer。
- [x] 1.4 增加协商契约测试，覆盖共同版本、缺少可选 capability、无共同版本、legacy Peer 不发 hello，以及激活前发送 state/action/PTY 消息。

## 2. Connection generation 与发送所有权

- [x] 2.1 将 Peer registry entry 重构为拥有 identity、单调 generation、connection context、私有 sender、pending requests 和所属 PTY 的生命周期对象。
- [x] 2.2 实现协商成功后的原子 generation 替换，并用 generation compare-and-swap 限制 cleanup；取消旧 entry 和等待 goroutine 必须在 registry lock 外完成。
- [x] 2.3 用 `PeerConnection.Send(ctx, message)` 封装有界发送队列和单一 WebSocket writer，明确返回 `queue-full`、`peer-offline` 或 stale-generation error，禁止调用方取得或关闭 channel。
- [x] 2.4 增加 generation 并发测试，覆盖快速重复重连、旧 handler 延迟退出、send 与 replace 竞争，以及旧 cleanup 不得关闭新连接或把新 host 标记 offline。

## 3. Correlated request 与幂等结果

- [x] 3.1 实现 Hub request tracker，使 accepted ack 保持非终态，并让 result、error、deadline、断连、撤销和本地发送失败只产生一次 terminal outcome；late 或重复响应只记录诊断信息。
- [x] 3.2 实现 Agent request dispatcher，在副作用执行前验证 generation、target、capability 和 deadline，并把 tmux 成功或失败转换为唯一 correlated result/error。
- [x] 3.3 实现基于 `request_id` 与规范化 payload digest 的有界 outcome cache：默认 TTL 5 分钟、每 Peer 1024 项、单个序列化 result/error 64 KiB，三项均可配置，并支持 in-flight 合并、完成结果重放、`request-conflict` 和 `resource-exhausted`。
- [x] 3.4 增加 request 生命周期测试，覆盖 queue full、ack 后 timeout、断连、旧 generation late response、重复 terminal response、缓存边界，以及 Agent instance ID 改变后返回 `execution-unknown` 且不重放副作用。

## 4. SessionTarget、Router 与调用方迁移

- [x] 4.1 定义不可变 `SessionTarget{host_id, session}` 及边界校验，并让所有新的 mutation API 强制显式携带 `host_id` 与 session，不保留缺省本机兼容路径。
- [x] 4.2 实现统一 Router 与 LocalExecutor/PeerExecutor contract，只在入口解析一次 target，并让 remote route 固定绑定当前 generation，任何失败均不得回退到 Hub 本机。
- [x] 4.3 迁移 rename、new、select-window、select-pane 及其余 session mutation handler，等待 terminal outcome 后再成功，并集中映射 `invalid-target`、`not-found`、offline、incompatible、timeout 和 execution error。
- [x] 4.4 更新 `web/src` 的 session/terminal hooks 与组件调用方，使每次 mutation 和 PTY open 都发送当前显式 `host_id` 与 session，并正确展示 structured error。
- [x] 4.5 增加 Router、API 与前端调用测试，覆盖本地/远端同名 session、缺失或未知 host、Peer offline、generation 切换、remote rename 返回新 target，以及绝不执行本机 fallback。

## 5. 运行时撤销与 Unix socket

- [x] 5.1 在 `pkg/socket` 与 Hub 定义用户私有 Unix socket 的 runtime revoke 命令、响应和权限校验，使运行中的 Hub 成为 Peer 授权状态唯一写入者。
- [x] 5.2 实现 Hub 撤销提交顺序：先原子持久化 Peer store，再更新内存授权，并取消目标 identity 的 active connection、pending requests、pending/active PTY。
- [x] 5.3 更新 `peers remove` 优先调用运行中 Hub 的 Unix socket；仅在 Hub 未运行时直接原子修改 store，并让在线与离线路径返回一致结果。
- [x] 5.4 增加撤销测试，覆盖持久化失败不产生部分撤销、live revoke 立即完成 pending request/PTY、当前连接退出，以及同一 identity 无需重启即被拒绝重连。

## 6. PTY v1 framing 与 resize

- [x] 6.1 实现 PTY v1 binary data frame codec，固定校验 magic、frame version、data direction、`uint64` sequence 和 payload 长度，同时保留原始 terminal bytes。
- [x] 6.2 实现 UTF-8 JSON control frame codec，校验 `version`、`type`、`sequence`，以及 resize 的正数 `cols`/`rows` 和 close/error 的受限 reason。
- [x] 6.3 更新 Hub browser bridge 与 Agent relay，把 browser binary input/text resize 转换为对应 v1 frame，让 resize 实际作用于远端 PTY，并保证 output 只返回同一绑定 stream。
- [x] 6.4 增加 framing 与 stream 顺序测试，覆盖畸形 header/JSON、重复 input sequence、sequence gap、非法尺寸、正确远端 resize，以及不得把 input/output 路由到其他 host 或 session。

## 7. PTY 拥有型生命周期与回收

- [x] 7.1 将每个 remote PTY 建模为绑定 identity、generation、SessionTarget 和一次性 attach token 的 owner，并在 15 秒 pending deadline 后拒绝 late data connection。
- [x] 7.2 实现 `Teardown(reason)` 的 `sync.Once` 顺序：先取消 context 并关闭 WebSocket/PTY fd 解除阻塞，再等待 relay goroutine，并在宽限期后终止和回收 tmux 子进程。
- [x] 7.3 将 browser、Agent data channel、control generation、timeout、replace、revoke、PTY I/O 和子进程退出全部接入同一 teardown；PTY open 仅在 Agent 创建且 data channel 绑定后成功。
- [x] 7.4 增加 PTY 生命周期测试，覆盖 idle PTY 时 browser disconnect、late attach、control connection 断开、replace/revoke、并发重复 close，以及无 goroutine、fd 或 tmux attach 子进程泄漏。

## 8. 故障注入、race、集成与 E2E

- [x] 8.1 建立可控 fake connection、fake tmux executor、时钟和同步 barrier，能够确定性触发背压、断连、延迟响应、late attach 与执行结果未知。
- [x] 8.2 在 `go test -race` 下覆盖 replace/send/revoke/cleanup、request timeout/result 和多方 teardown 的关键交错，并修复发现的数据竞争或 deadlock。
- [x] 8.3 增加 Hub-Agent 集成测试，贯通认证后协商、状态激活、远端 mutation 成功/失败与去重、live revoke、PTY 建立/late arrival/input/resize/断连。
- [x] 8.4 增加浏览器 E2E，验证显式 host+session 的远端 rename/new/select/terminal input、structured error 展示、resize，以及 idle terminal 断开后资源被回收。

## 9. 升级说明与发布门禁

- [x] 9.1 更新 multi-host、Agent support matrix 与发布说明，记录 breaking protocol、Hub 先于 Agent 的升级顺序、embedded frontend 同版本迁移、无 `host_id` fallback、兼容性诊断和成套回滚要求。
- [x] 9.2 执行并记录发布门禁：Go 单元/集成测试、目标包 `go test -race`、前端测试与构建、相关 E2E，以及 `openspec validate stabilize-multi-host-peer-runtime --strict` 全部通过。

  2026-07-26 发布门禁记录：
  - `go test ./...`
  - `go test -race ./pkg/peer ./pkg/server ./pkg/socket ./pkg/commands/peers ./pkg/tmux`
  - `cd web && npm run build`
  - `cd web && npx playwright test`（6 项 E2E）
  - `openspec validate stabilize-multi-host-peer-runtime --strict`

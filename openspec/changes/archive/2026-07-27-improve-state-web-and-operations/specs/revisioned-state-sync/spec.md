## ADDED Requirements

### Requirement: Canonical single-writer state projection
Hub SHALL 通过一个 single-writer coordinator 提交所有面向 Web UI 的 session、host、tool event、activity 和 health 状态变化。每个 producer SHALL 提交 typed mutation，且网络 I/O 或 subscriber 写入 MUST NOT 在 coordinator 的提交临界区内执行。

#### Scenario: Concurrent producers update state
- **WHEN** 本机 discovery、Peer 聚合和 tool tracker 并发提交 mutation
- **THEN** coordinator 按一个确定的提交顺序应用变化，并向所有 reader 暴露同一 canonical result

#### Scenario: Mutation does not change material state
- **WHEN** producer 提交的 mutation 与当前 canonical state 等价
- **THEN** coordinator 不产生新的 committed revision

#### Scenario: Slow subscriber cannot block state writer
- **WHEN** 一个浏览器 subscriber 无法及时消费 delta
- **THEN** coordinator 继续提交后续 mutation，并把该 subscriber 转入 resync 路径而不是阻塞全局 state writer

### Requirement: Stable identities in canonical state
Canonical projection SHALL 使用 stable host ID 和包含 host identity 的 session/window/pane key。Display name SHALL 仅作为可变展示字段，MUST NOT 作为跨主机 session 的操作目标、持久化 key 或唯一 React key。

#### Scenario: Two hosts contain the same session name
- **WHEN** 两个不同 host ID 都报告名为 `work` 的 session
- **THEN** canonical projection 保留两个独立 session identity，任一 session 的选择或 mutation 不影响另一项

#### Scenario: Two hosts share a display name
- **WHEN** 两个不同 host ID 使用相同 display name
- **THEN** state 和 UI 仍将它们作为两个独立 host 展示和寻址

### Requirement: Instance and revision state identity
Hub SHALL 为每次 process lifetime 生成 opaque `instance_id`，并在该 instance 内为每次 material commit 分配单调递增 revision。Snapshot 和 delta SHALL 同时携带 state schema version、instance identity 和 revision metadata。

#### Scenario: State changes within one Hub instance
- **WHEN** coordinator 连续提交两个 material mutations
- **THEN** 第二次提交的 revision 严格大于第一次，并且两个 envelope 的 `instance_id` 相同

#### Scenario: Hub process restarts
- **WHEN** Hub 重启并重新构建 canonical projection
- **THEN** 新 snapshot 使用新的 `instance_id`，客户端不把新 revision 与旧 instance 的 revision 直接比较

### Requirement: Atomic snapshot and delta subscription
状态 WebSocket SHALL 从 coordinator 原子取得 subscriber registration 和对应 revision 的完整 snapshot，并在该 snapshot 之后按提交顺序发送 delta。Snapshot 与首个 delta 之间 MUST NOT 存在未表达的 state change。

#### Scenario: Browser establishes its first state connection
- **WHEN** 受支持的浏览器连接状态 WebSocket
- **THEN** Hub 先发送包含完整 projection 的 snapshot，再发送以该 snapshot revision 为 base 的后续 delta

#### Scenario: State changes while snapshot is being delivered
- **WHEN** coordinator 在 WebSocket handler 写出初始 snapshot 期间提交新 mutation
- **THEN** 对应 delta 已由同一 subscriber 排队，并在 snapshot 后按 revision 顺序送达

#### Scenario: Subscriber queue overflows
- **WHEN** bounded subscriber queue 无法容纳后续 delta
- **THEN** Hub 明确要求该客户端 resync 或关闭该状态连接，而不是静默丢弃增量后继续宣称同步

### Requirement: Ordered delta application and gap recovery
Web UI SHALL 在单一 reducer transaction 中验证并应用 state delta。客户端 SHALL 忽略重复或旧 revision；检测到 instance change、`base_revision` mismatch、revision gap 或无法应用的 operation 时 SHALL 放弃未确认增量并取得新的完整 snapshot。

#### Scenario: Next ordered delta arrives
- **WHEN** delta 的 instance 与客户端相同且 `base_revision` 等于客户端当前 revision
- **THEN** reducer 原子应用全部 operations，并把客户端 revision 推进到该 delta revision

#### Scenario: Duplicate delta arrives
- **WHEN** delta revision 小于或等于客户端当前 revision
- **THEN** 客户端忽略该 delta，且 projection 不发生第二次变化

#### Scenario: Revision gap is detected
- **WHEN** delta 的 `base_revision` 不等于客户端当前 revision
- **THEN** 客户端停止把后续 delta 应用于旧 projection，并进入 rehydrating 状态直到完整 snapshot 成功应用

#### Scenario: Hub instance changes
- **WHEN** 重连后的 envelope 使用不同 `instance_id`
- **THEN** 客户端替换整个 projection，而不是尝试把新 instance 的 revision 合并到旧 state

### Requirement: State schema compatibility handling
浏览器状态连接 SHALL 声明其支持的 state schema。Hub SHALL 对受支持 schema 提供 snapshot/delta，并对不兼容 schema 返回明确的 reload-required outcome，MUST NOT 让不兼容客户端无限按普通网络错误重连。

#### Scenario: Browser supports current schema
- **WHEN** 浏览器声明的 schema 与 Hub 当前 state schema 兼容
- **THEN** Hub 建立正常 snapshot/delta stream

#### Scenario: Loaded frontend is too old
- **WHEN** 浏览器声明的 schema 不在 Hub 支持范围内
- **THEN** Hub 返回 reload-required，Web UI 提示或执行受控 reload 以取得当前嵌入前端

### Requirement: Single authoritative frontend state provider
Web UI SHALL 使用一个 application state provider 保存 snapshot、revision metadata 和 normalized projection。Session、host、tool event、activity 与 health consumer MUST NOT 再以独立轮询响应直接覆盖权威 projection。

#### Scenario: An older HTTP response arrives after a delta
- **WHEN** 迁移期读取请求在更新后的 canonical delta 之后返回旧数据
- **THEN** 旧响应不会覆盖 provider 中较新 revision 的 state

#### Scenario: Component selects derived state
- **WHEN** Sidebar、Overview、StatusBar 或 Fleet Health 需要 session/host 数据
- **THEN** 组件通过 provider selector 读取相同 revision 的 projection

### Requirement: Ghost-free browser WebSocket lifecycle
每个挂载的 browser state connection controller SHALL 至多维护一个当前 socket 和一个 reconnect timer，并使用 generation/disposed guard 使旧 socket handler、过期 timer、StrictMode cleanup、`visibilitychange` 与 `pageshow` 无法创建 ghost connection。

#### Scenario: React StrictMode remounts an effect
- **WHEN** development StrictMode 执行 effect setup、cleanup 和再次 setup
- **THEN** cleanup 后的旧 socket close handler 不会安排重连，最终只有新 generation 的一个连接

#### Scenario: Component unmounts or user signs out
- **WHEN** connection controller 被 dispose
- **THEN** 它取消 timer、注销 handlers、关闭当前 socket，并且此后不会再创建连接

#### Scenario: Page becomes visible while socket is connecting
- **WHEN** `pageshow` 或 `visibilitychange` 在当前 generation 已有 CONNECTING socket 时触发
- **THEN** controller 不创建第二个 socket

### Requirement: Reconnect rehydrates before readiness
Browser state connection SHALL 使用有上限且带 jitter 的 exponential backoff 重连。每次新连接 SHALL 在成功应用完整 snapshot 后才进入 ready 状态；连接打开本身不足以宣告 state 已同步。

#### Scenario: Network returns after missed changes
- **WHEN** 浏览器断线期间 Hub state 已发生变化，随后 WebSocket 重连
- **THEN** UI 先显示 rehydrating，应用当前 snapshot 后才显示 ready，并包含断线期间的全部当前状态

#### Scenario: Reconnect repeatedly fails
- **WHEN** Hub 在多个 reconnect attempt 中仍不可达
- **THEN** attempt 按 capped backoff 调度，UI 显示 reconnecting，并且同时最多存在一个 pending timer

#### Scenario: Rehydration succeeds
- **WHEN** 新 snapshot 被完整验证和应用
- **THEN** controller 清零 backoff、记录新的 instance/revision，并将连接标记为 ready

### Requirement: Revisioned state synchronization verification
项目 SHALL 包含自动化测试，覆盖 single-writer ordering、atomic subscription、revision gap、subscriber overflow、schema mismatch、StrictMode cleanup、visibility/pageshow 去重和 reconnect rehydrate。

#### Scenario: State synchronization test suite runs
- **WHEN** CI 执行 Go state tests、frontend Hook tests 和 browser reconnect tests
- **THEN** suite 验证并发 producer 不破坏 revision 顺序，且任何 lifecycle 路径都不会留下 ghost socket 或 stale authoritative state

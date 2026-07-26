## Context

TmuxAtlas 的浏览器界面目前从多个彼此独立的来源维护状态：session、host、tool event、activity 和 stats 分别通过首次 HTTP 请求、周期轮询及 WebSocket 增量更新。各 Hook 都可以直接覆盖自己的 React state，响应之间没有统一顺序、状态世代或 revision。浏览器断线期间漏掉增量后，重新建立 `/ws/events` 只收到版本 welcome，不会取得完整快照；旧 socket 的 `onclose` 和 timer 也可能在组件卸载或新连接建立后继续重连。

Hub 已经从 Peer 和本机收集稳定 host ID、版本、session、activity 与部分健康事实，但 Web UI 仍在若干位置用显示名或 session name 作为身份，并且没有 Fleet 级别的版本与健康视图。本设计会扩展浏览器可见的聚合状态，但不改变 Peer authentication、pairing、heartbeat、重连或连接管理；Peer 产生的事实只作为 canonical state coordinator 的输入。

Web Push 使用 root-scoped、network-neutral Service Worker，但 subscription 只存在于 Hub 内存中。Hub 重启后，关闭状态的 PWA 无法收到通知；服务端 sender 也没有执行通知偏好，payload 未携带足够的 host 身份。现有 `progressive-web-app` capability 已定义 Service Worker 和 Push 展示，因此 subscription durability、reconciliation、host-aware navigation 与 preference enforcement 将作为该 capability 的增量，而不是另建一套通知生命周期。

Self-updater 已能下载 release、校验 SHA-256、原子替换当前 executable 并重启 systemd/launchd 服务，但当前同源下载的 `checksums.txt` 本身没有经过 provenance 验证；release 已发布 `checksums.txt.sigstore.json`，updater 却未消费它。“restart 命令成功”也不等于新版本健康，替换失败或新进程不能 ready 时没有 last-known-good rollback。与此同时，移动 PWA 缺少终端 modifier/clipboard 工具栏和 safe-area 布局，前端字体仍在运行时访问 Google Fonts，导致隔离网络部署的呈现不可复现。

当前 `tmuxatlas server` 还在入口无条件创建 tmux client，并启动 discovery、control mode、进程树 detector、silence monitor 与本机 PTY executor。这个 standalone composition 适合直接安装在有 tmux 的机器上，却不能作为安全的 Hub-only 容器：挂载宿主机 tmux socket 仍不能正确看到宿主 PID namespace，继续要求 `pid: host`、privileged 或 Docker socket 又会破坏隔离。普通服务器上的 Docker 部署因此必须先拆出不依赖本机 tmux 的纯 Hub role，而不是给现有 `server` 外包一层镜像。

## Goals / Non-Goals

**Goals:**

- 让一个 Hub process 内所有浏览器可见状态通过单一写入者提交，并以 instance identity 与单调 revision 表达顺序。
- 让浏览器在首次连接和每次重连时，从与后续 delta 原子衔接的 snapshot 开始；检测 gap 或 schema 不兼容时确定性 rehydrate/reload。
- 保证 React StrictMode、页面隐藏/恢复、登出和组件卸载不会产生 ghost WebSocket 或重复 reconnect timer。
- 持久保存 Push subscriptions，统一执行通知偏好，并让远端通知稳定导航到正确 host/session。
- 提供以稳定 host ID 为基础的 Fleet Health、版本漂移和只读 remediation。
- 让 updater 先以 pinned Sigstore trusted root 和固定 GitHub Actions release identity 验证 checksum provenance，验证成功后才信任 checksum；新服务只有通过有界 health check 才提交更新，失败时恢复 last-known-good binary。
- 提供不构造 tmux client 或本机 executor 的纯 Hub role，同时保持 Agent 和现有 standalone server 行为清晰、可测试并向后兼容。
- 让普通 Linux 服务器可以用官方 non-root OCI image 和单服务 Compose 启动纯 Hub，以一个持久 volume 工作且不依赖 PostgreSQL、Redis 或其他外部状态服务。
- 让 Docker Hub 始终位于 Cloudflare Tunnel 或 Nginx+ACME 等可信 HTTPS 网关之后，并通过不可变 image pull/recreate 完成升级和回滚。
- 为移动终端提供触屏控制、safe-area 与软键盘适配，同时保持桌面 xterm 输入语义。
- 把 UI 字体和 Nerd symbols 纳入嵌入式构建、许可证清单与 bundle budget，消除运行时第三方字体请求。
- 通过并发、故障注入、Hook、Playwright、WebKit/mobile 和构建预算测试验证上述契约。

**Non-Goals:**

- 修改第一单定义的公网 Host/Origin、应用认证、TLS/反向代理或其他安全边界；Docker 只复用这些契约。
- 修改 Peer authentication、pairing、heartbeat、重连、连接生命周期或 PTY relay 协议。
- 支持 Cloudflare Containers/Durable Objects、Kubernetes、横向多 Hub writer、PostgreSQL、Redis 或外部数据库/cache。
- 把 Agent 放进官方 Hub image，或要求容器挂载宿主机 tmux socket、Docker socket、共享 PID namespace、使用 privileged/host network。
- 在 TmuxAtlas 或容器内终止 TLS、生成自签名证书或替代宿主可信网关。
- 从浏览器远程执行任意命令，或批量自动升级 Fleet。
- 将 terminal byte stream、scrollback 或认证状态纳入 canonical application state snapshot。
- 提供离线 terminal、离线 shell、API cache、后台 mutation queue 或 Service Worker application shell cache。
- 为任意旧版外部客户端永久维护浏览器状态协议；Hub 与嵌入前端仍按同一 release 发布。

## Decisions

### 1. 由单 goroutine coordinator 提交 canonical browser state

新增一个 Hub-owned state coordinator。session discovery、Peer 聚合、tool tracker、activity collector、preferences/updater health 等 producer 不再分别面向浏览器广播或让前端拼接状态，而是提交 typed mutation 给 coordinator。一个 goroutine 按接收顺序验证、归一化并提交 mutation；只有发生 material state change 的成功提交才增加 revision。

Canonical projection 中的集合使用稳定身份作为 key：host 使用 host ID，session 使用 `hostID/sessionName`，window/pane 使用其稳定 ID 或带父级身份的 composite key。显示名仅作为字段，绝不作为持久化、React key 或操作目标。Terminal 的二进制数据不进入 projection。

Reader 获得不可变 snapshot 或 defensive copy，网络写入和慢 subscriber 不得持有 coordinator 的写锁。与在每个现有 manager 上增加 revision 相比，单 coordinator 可以为跨 session/host/tool-event 的一次逻辑变化提供一个可观察提交顺序，并消除浏览器端多 store 覆盖。

### 2. `(instance_id, revision)` 共同定义状态世代与顺序

Hub 每次 process 启动生成新的 opaque `instance_id`，revision 在该 instance 内从初始 snapshot revision 开始单调递增。Revision 不持久化；重启后新的 instance identity 明确告诉客户端旧 revision 不可比较。

Snapshot envelope 至少包含 `schema_version`、`instance_id`、`revision` 和完整 projection。Delta envelope 至少包含 `schema_version`、`instance_id`、`base_revision`、`revision` 及 typed operations。一次 delta 必须从客户端当前 revision 精确推进到下一已提交 revision；revision 小于等于当前值的消息可按重复/旧消息忽略，`base_revision` 不匹配、instance 改变或 operation 无法应用时必须丢弃未确认增量并请求完整 rehydrate。

Schema incompatibility 不能被当成网络错误无限重试。浏览器在连接参数中声明支持的 state schema；Hub 对仍可服务的 schema发送 snapshot，对不兼容 schema 发送明确的 `reload-required`，让旧的已加载页面刷新到当前嵌入前端。

### 3. WebSocket subscribe 与初始 snapshot 在 coordinator 中原子衔接

浏览器状态连接不采用“先 HTTP snapshot、再独立订阅 WebSocket”的两步流程，因为两者之间会存在漏事件窗口。WebSocket handler 向 coordinator 提交 subscribe command；coordinator 在同一写入顺序点注册 bounded subscriber 并返回该 revision 的 immutable snapshot。Handler 先写 snapshot，再按 channel 顺序写后续 delta。

Subscriber channel 满载时 coordinator 不阻塞。Hub 向该客户端发出 `resync-required` 或关闭状态连接；客户端随后用新连接取得 snapshot。该机制把 backpressure 退化为可恢复的 rehydrate，而不是静默丢事件。

现有 `/api/sessions`、`/api/hosts` 等读取端点可在迁移期保留，并从同一 projection 派生；新 Web UI 的权威启动和重连路径只使用原子 snapshot/delta stream，避免轮询响应覆盖更新后的 reducer state。

### 4. 一个 browser connection controller 管理一个状态 socket

前端提供集中式 connection controller，并使用递增 generation 与 disposed flag。创建新 socket 前取消旧 timer、注销旧 handlers 并关闭旧 socket；任何 handler 或 timer 执行前都验证 generation。组件 cleanup 先标记 disposed，再解除 handler 和关闭连接，因此 close 事件不能重新调度。

`visibilitychange` 与 `pageshow` 只在当前 generation 没有 OPEN 或 CONNECTING socket 且没有已安排 timer 时触发连接。重连使用带 jitter 的 capped exponential backoff，成功接收并应用 snapshot 后重置 backoff。连接状态区分 `connecting`、`rehydrating`、`ready`、`reconnecting`、`auth-required` 与 terminal error，只有 projection 与 server revision 一致时才显示 ready。

该 controller 可复用 generation/timer cleanup 原语给 browser terminal WebSocket，但 revisioned rehydrate 仅属于状态 channel；本设计不改变 Peer WebSocket lifecycle。

### 5. 前端使用单一 reducer 应用 snapshot 与 delta

新增 application state provider，集中保存 envelope metadata 和 normalized projection。Snapshot 是一次替换；delta 在一个 reducer transaction 中完成验证和应用。派生 selector 替代 `useSessions`、`useHosts`、`useToolEvents`、`useActivity` 各自的权威本地副本，组件不能直接以任意 fetch response 覆盖 projection。

用户发起的 mutation 仍通过 HTTP API 提交，服务端成功提交后由 canonical delta 确认。UI 可以显示 pending operation，但不凭 HTTP `2xx` 假定远端 session 已经出现；目标创建、rename 等操作以稳定 host/session identity 等待相应 delta，或在超时后显示可重试错误。

### 6. Push subscription 使用原子持久 store，并由浏览器与 Hub reconciliation

Push store 使用配置目录中的专用 JSON 文件，文件权限为 `0600`，按 endpoint 去重并保存浏览器提交的 endpoint、P-256 key、auth secret 及必要时间戳。写入使用同目录 temporary file、`fsync` 和 rename；Hub 启动时加载 store，404/410 等永久失效响应继续原子删除 endpoint。

受支持浏览器在已认证应用启动时注册/取得同源 Service Worker，读取现有 `PushManager` subscription，并与 Hub reconcile。只有 Hub subscribe API 返回成功后 UI 才进入 `subscribed`；网络或服务器失败显示可重试状态并保留本地 subscription 以便再次登记。取消订阅也必须反映 Hub 与浏览器两侧的实际结果，而不是无条件宣告成功。

历史版本没有持久 subscription 文件，因此首次升级以空 store 启动；当前打开的浏览器会自动登记，已关闭浏览器需下一次打开后登记一次。这是从不可恢复内存状态到 durable store 的 additive migration。

### 7. Push sender 在服务端执行偏好并携带完整目标身份

Sender 在投递每个 tool event 时读取当前 `preferences.notifications.statuses`，Waiting、Error 与 Completed 使用同一服务端决策；页面内 Notification 只作为没有有效 Push subscription 时的 fallback，不能形成另一套偏好语义。

Push payload 包含 stable host ID、host display name、session、window、pane、tool、status、title 与 body。Notification tag 至少包含 host/session/window/pane/tool，避免不同主机的同名 session 相互覆盖。Service Worker 逐段 `encodeURIComponent` 构造 `/session/<host>/<session>` 目标，并继续拒绝跨 origin URL。缺少必需身份或 malformed payload 时退回 Hub root，不猜测本机 session。

Service Worker 继续没有 `fetch` handler 和 application cache；subscription persistence 不改变 network-only PWA 决策。

### 8. Fleet Health 表达事实与 reason codes，不用显示名推断

每个 host health record 以 host ID 为主键，包含 display name、role、online observation、last seen、last state sync、application version/commit、state schema 或显式 compatibility metadata、Agent/hook checks，以及最近 updater outcome。Agent 可通过既有 state/stats 更新携带附加 health facts，但这不改变连接建立、心跳或重连方式。

Health model 保留原始 facts，并计算可组合的 reason codes，而不是只保存一个会丢信息的总状态：

- `unknown`：缺少判断所需 metadata；
- `stale`：状态同步年龄超过配置的 health freshness threshold；
- `offline`：既有 host observation 表示离线；
- `version-behind` / `version-ahead`：可解析的应用版本分别低于或高于 Hub；
- `incompatible`：显式 state/protocol compatibility range 不包含 Hub 支持版本，不能仅凭应用版本猜测；
- `healthy`：在线、fresh、显式兼容且没有失败的 Agent/hook check。

UI 显示最高严重度摘要，同时允许查看全部 reason、原始版本、年龄和 check。Remediation 仅提供 role/platform 适用的诊断或更新命令及 copy action；Fleet 页面不执行命令。两个相同 display name 的 host 始终保持独立。

### 9. Updater 在信任 checksum 前验证 Sigstore release provenance

Updater 同时下载目标 release 的 `checksums.txt` 与 `checksums.txt.sigstore.json`，直接链接 Go Sigstore verifier，不 shell out 到 `cosign`。Verifier 使用随 binary 固定并版本化的 trusted-root snapshot 校验 bundle、签名证书和透明日志材料，再执行固定 identity policy：OIDC issuer 必须等于 `https://token.actions.githubusercontent.com`，repository 必须等于 `LosFurina/tmuxatlas`，workflow 必须等于 `.github/workflows/goreleaser.yml`，证书 workflow identity/ref 必须精确绑定 release API 已选定的 `refs/tags/<target-tag>`。目标 tag 由 updater 先确定，不能从 bundle 反向接受。

只有 bundle 对下载到的 checksum bytes 验证成功且 trusted root、issuer、repository、workflow、tag 全部匹配，updater 才解析并使用 checksum 校验 archive。Bundle 缺失、malformed、签名或透明日志验证失败、identity 不匹配均在 staging/replace/restart 前 fail closed；同一 GitHub origin 不是信任依据。

Pinned root 使离线验证可复现，但 Sigstore root rotation 可能让未来合法 release 被旧 updater 拒绝。Root 更新必须作为受审查的源码/依赖变更随新版本发布，绝不在验证失败时回退到仅 checksum；checked-in fixtures 覆盖有效 bundle、issuer/repository/workflow/tag 错配、缺失/损坏 bundle和 root rotation 前后边界。

### 10. Updater 使用 last-known-good binary 与 transaction journal

Updater 在目标 executable 同一文件系统创建 staged binary，完成 checksum、解包、权限、可执行性和 `fsync` 后，再复制当前 executable 为 last-known-good backup 并原子 rename staged binary 到目标路径。最多保留一个已知可启动的 previous binary 与对应版本 metadata。

配置目录保存小型 update transaction journal，记录 source/target version、executable、service role/name、backup、阶段和最后错误。Journal 的阶段至少区分 staged、replaced、restarted、healthy、rolling-back、rolled-back。成功 health check 后清理临时 staging 与进行中的 transaction，但保留一份 previous release 供显式手动 rollback；新的成功升级可以轮换它。

若 updater 在未提交状态中断，下次 `tmuxatlas update` 或显式 recovery/rollback 命令先检查 journal，并在继续新升级前给出确定的 resume/rollback 结果。Updater 不静默删除无法解释的 backup。

### 11. 所有 runtime role 使用本地、模式化 readiness probe

Pure Hub、standalone 和 Agent 的本地 Unix HTTP listener 提供轻量 health response，至少包含 role、deployment mode、version、commit、ready 和 process instance。Pure Hub ready 表示 canonical state coordinator 与必需 Hub core loops 已启动；standalone 还验证 local integration；Agent ready 表示本地 socket 和 Agent 主循环已启动。Probe 不取代公网健康检查，也不改变 Peer liveness。

Updater 从已验证的 service definition、arguments 和 environment 解析 role 与 socket path，重启后在 bounded timeout 内轮询本地 probe，并同时确认 service manager 报告 active。返回的 role 与目标模式、version 与 target release 必须匹配。Restart 命令失败、服务退出、probe 超时、role/version 不匹配或 `ready=false` 都触发自动 rollback：原子恢复 previous binary、再次 restart，并验证 previous version ready。

自动 rollback 成功仍使原更新命令以非零状态退出并清楚报告新版本失败、旧版本已恢复。Rollback 也失败时保留 journal/backup，返回更高严重度的可操作错误。`--no-restart` 继续只替换 binary，不宣告运行中服务已升级，也不自动执行运行时 health check。

### 12. 移动 Terminal 工具栏发送确定的 xterm 输入

在 touch/coarse-pointer 或用户显式启用时展示可收起工具栏。工具栏至少提供 Esc、Tab、Ctrl、Alt、方向键、Copy、Paste 与软键盘 show/hide。Esc、Tab 和方向键发送与物理键一致的 terminal sequence；Ctrl/Alt 可以作为 one-shot modifier 与下一次可组合按键一起编码，并提供清除/取消状态。锁定 modifier 时必须有视觉和可访问状态，session/host 切换、断线或 toolbar 收起不得把未使用 modifier 意外发送到新目标。

Copy 仅复制当前 xterm selection，并明确反馈成功/失败；Paste 只在用户 gesture 中读取 clipboard，在获得文本后一次发送到当前 generation 的 OPEN terminal connection。权限失败、空 clipboard 或连接已变化不得发送旧文本。浏览器 native clipboard API 不可用时提供受支持的 fallback 或可理解错误，而不是静默失败。

### 13. 使用 Visual Viewport 与 safe-area 适配移动布局

应用根布局为 top bar、terminal/content、可选 toolbar 与 status 区域应用 `env(safe-area-inset-top/right/bottom/left)`。窄屏 sidebar 变为可关闭 drawer，不永久占用 terminal 宽度；modal/quick switcher 使用 viewport-relative 最大尺寸。

监听 `visualViewport` 或等价能力处理软键盘造成的可见高度与 offset 变化，并在 settle 后 coalesce xterm fit/resize，避免 resize event storm。触屏按钮满足至少 44×44 CSS px 的 target，具备 accessible name、pressed/checked 状态和键盘等价操作。桌面布局、物理键盘直通、selection 和 scrollback 语义保持不变。

### 14. 字体与 Nerd symbols 随嵌入前端发布

移除 Google Fonts CSS 与 preconnect。实际使用的开源 UI/terminal 字体以 WOFF2 放入前端资产，并随 release 保存许可证和来源记录。专有或平台字体只列为明确的 system font；设置界面标记 bundled/system，不能把未加载且通常不存在的字体宣称为 bundled。

使用 Nerd Fonts Symbols Only 或记录过来源与许可证的最小 glyph subset 作为 fallback family，使 Private Use Area 符号不要求主字体为 patched Nerd Font。Subset 生成必须可复现，并有 glyph inventory 测试；普通文本仍由首选 monospace 渲染。

默认首屏字体可以 preload，并使用合适的 `font-display`；其他 terminal 字体和 xterm code 在实际打开 terminal/选择字体时加载。版本控制的 bundle budget 文件记录 gzip/Brotli 后的入口 JS/CSS、lazy xterm chunk 和关键字体上限，CI 超限必须显式更新预算及说明，不能静默接受。

### 15. 测试以真实边界和故障注入验证恢复路径

Go 测试使用并发 producer 和 bounded subscriber 验证 single-writer、revision、atomic subscribe、gap/overflow、snapshot 一致性、Push store restart、preference filtering、host payload，以及 Sigstore provenance policy 与 updater 各 transaction/rollback 阶段。Provenance tests 使用 checked-in valid/invalid bundle fixtures；service updater tests 使用临时 executable 和 fake systemd/launchd/health probe，不操作开发机服务。

React/Hook 测试启用 StrictMode 与 fake timers，验证 mount/unmount、visibility/pageshow、CONNECTING 去重、generation guard、backoff cleanup、snapshot/delta reducer 和 stale HTTP response 不再成为权威状态。

Playwright 启动隔离 Hub/fixture hosts，覆盖两个 host 的同名 session、断线期间 mutation 后 rehydrate、真实应用 Service Worker 注册、Hub restart 后 subscription reconciliation、host-aware notification navigation、update/reload 提示、移动 toolbar、clipboard failure、safe-area 和 modal accessibility。Chromium 覆盖 install/Push 相关行为，WebKit mobile project 覆盖 iPhone/iPad 布局和触摸输入。构建测试禁止第三方字体请求并执行 version-controlled bundle budgets。

### 16. 纯 Hub 与 standalone 共享 Hub core，但隔离 local producer

启动 wiring 明确组合三个角色：

- `tmuxatlas hub` 只构造 Hub core、Web/Passkey、Push、Peer registry/handler、canonical state、远程 PTY relay 和本地管理/health socket，不查找 tmux，也不启动 discovery、control mode、tool detector、silence monitor、本机 activity/stats producer 或本机 PTY executor。
- `tmuxatlas agent` 保持 outbound-only，继续在实际拥有 tmux 的宿主机上运行。
- `tmuxatlas standalone` 组合 Hub core 与 local producer/executor；现有 `tmuxatlas server` 保留为该角色的兼容入口，避免已有 systemd/launchd 安装静默改变语义。

Hub core 通过显式 interface/optional capability 接受 local producer 和 executor，而不是靠可空 `tmux.Client` 到处做条件分支。纯 Hub 的 projection 可以没有本机 session host；引用 Hub identity 作为 tmux target、缺失 host 或没有在线 Agent 的 action 必须明确返回 unsupported/not-found，绝不能在容器内尝试 tmux 或回退到其他 Agent。

纯 Hub 在尚无 Passkey、尚无配对 Agent 或全部 Agent offline 时仍是 process-ready，因为它已经能够服务 setup、管理和连接入口；Fleet/application readiness 另外表达 setup-required 与无在线 Agent。`install --mode hub`、doctor、Unix health response 和 service metadata 使用显式 role，不以 tmux 是否存在推断运行模式。SIGTERM 先停止接收新 HTTP/WS/PTY，再有界关闭 browser/Peer connections、Push sender、coordinator 与本地 socket。

### 17. Docker 是普通服务器上纯 Hub 的安全包装

官方 image 使用 multi-stage build 生成包含嵌入前端的静态 Go binary，runtime layer 只保留 binary、系统 CA、non-root 用户和必要的 OCI metadata，不安装 tmux、shell、编译器或 package manager。默认 command 为 `tmuxatlas hub`。镜像与 Compose 不使用 privileged、host PID/network、额外 Linux capabilities、Docker socket或宿主 tmux socket，并兼容 read-only root filesystem、`no-new-privileges` 和临时 `/tmp`、`/run/tmuxatlas`。

单服务 Compose 在容器内监听 `0.0.0.0:7654`，默认只发布到宿主 `127.0.0.1:7654`。`TMUXATLAS_PUBLIC_URL` 是最终浏览器 HTTPS origin 且必须显式提供；Cloudflare Tunnel 或 Nginx+ACME 从宿主 loopback 反代 HTTP/WSS，保留第一单要求的 authority/Host/Origin。容器不拥有证书、不生成自签名 TLS，也不默认信任 forwarded client-address headers。

服务器本身若也有需要纳管的 tmux，会在宿主机另装 systemd Agent，再像其他机器一样通过最终 HTTPS URL 配对。容器只做 Hub，这避免 PID namespace、tmux socket ownership 和容器内 PTY 与宿主执行环境不一致。

### 18. 一个本地 volume 承载单 Hub 的文件状态

Compose 挂载一个 named volume 到 `/var/lib/tmuxatlas`，并设置：

```text
XDG_CONFIG_HOME=/var/lib/tmuxatlas/config
XDG_DATA_HOME=/var/lib/tmuxatlas/data
TMUXATLAS_SOCKET=/run/tmuxatlas/tmuxatlas.sock
TMUXATLAS_DEPLOYMENT=docker
```

`paths.ConfigDir` 补齐标准 `XDG_CONFIG_HOME` 支持；既有 Passkey credentials、Hub identity、Peer trust、preferences、durable Push subscription、VAPID 与 updater/outcome store 继续按各自 ConfigDir/DataDir 契约落入对应 XDG subtree，而两个 subtree 都位于同一 volume。所有文件延续 non-root ownership、`0700` 目录、`0600` secret 和同文件系统原子 write/rename 契约。Compose 与文档不提供无 volume 的生产示例，启动/doctor 对不可写或明显临时的数据路径给出可操作失败。

该方案只支持一个 active Hub process/replica；JSON/file stores 是单写入者持久化，不声称提供分布式锁或多副本一致性，也不引入 PostgreSQL、Redis。Online Agent、活动 PTY、canonical revision 和 browser Session token 保持 process-scoped；容器重建后 Agent 自动重连、projection 重建，浏览器需要再次用已保留的 Passkey 登录，即使原 idle TTL 尚未届满。

首次 bootstrap secret 在 Docker 部署中不写入 image、Compose environment 或长期普通日志。操作者通过 `docker compose exec tmuxatlas` 调用同用户 Unix-socket 管理命令取得/轮换短期 token；配对码同样通过容器内 CLI 调用正在运行的 Hub。

### 19. 容器使用不可变 image 更新，原生安装继续事务式 self-update

Dockerfile 设置可识别的 deployment mode 与 build version/image revision。`tmuxatlas update --check` 可以只读查询新版本；任何 install、replace、restart 或 rollback binary 的路径在 `TMUXATLAS_DEPLOYMENT=docker` 下都 fail closed，不触碰 image filesystem，并输出 `docker compose pull && docker compose up -d`、health 检查及 pin previous SemVer tag/digest 的回滚步骤。Doctor 和 Fleet remediation 根据 deployment mode 显示 image workflow，而不是 systemd/launchd binary updater 命令。

Native systemd/launchd 的 updater 保留 Sigstore provenance、staging、health commit 与 binary rollback 语义。Docker 运维则在 recreate 前备份 volume，拉取固定 SemVer tag或 digest，重建后等待 container health；失败时以 previous tag/digest 重建，绝不使用 `docker compose down -v` 作为升级步骤。生产文档可以展示 mutable `latest` 用于发现，但部署与 rollback 示例必须固定版本或 digest。

Tag release 使用仓库 `GITHUB_TOKEN` 的最小 `packages: write` 权限发布 `linux/amd64` 和 `linux/arm64` GHCR image，不需要个人 full-access token。Release 产生 OCI labels、SBOM、provenance 和 keyless signature；PR/branch CI 只 build、inspect 与 smoke test而不 push。已发布 SemVer image 不覆盖，stable release 可另行移动 convenience tag。

### 20. Container validation 覆盖 role、隔离、持久化和真实 Agent

CI 在没有 tmux 的 runtime image 中启动纯 Hub，验证 non-root UID、read-only root、capability drop、loopback publish、Unix health command、setup-required readiness 与有界 SIGTERM。`docker compose config` 必须解析必填 Public URL、单 service、单 volume、healthcheck、restart policy 和 grace period，且不得出现 external database/cache、privileged、host PID/network 或敏感 socket mount。

集成测试启动 Hub container 和运行真实 tmux fixture 的宿主/测试 Agent，完成 bootstrap Passkey、pairing、state sync、远端 PTY/input/resize、Agent disconnect/reconnect。测试随后保留 volume recreate Hub，验证 Passkey、Hub identity、Peer trust、preferences、VAPID 与 Push subscription 不变，Agent 重连成功，并明确断言旧 browser Session 已失效。

Release gate 拉取或加载目标 architecture image，验证 embedded version/commit、OCI/SBOM/signature metadata和 health；反向代理 E2E 继续覆盖 Cloudflare Tunnel/Nginx 等价的 HTTP/WSS forwarding，但不在 Docker change 中重新定义入口安全策略。

## Risks / Trade-offs

- **Canonical projection 扩大单写入者职责** → Producer 只提交短小 typed mutation，writer 不执行 I/O；snapshot 使用 immutable/copy-on-write 数据，subscriber backpressure 通过 resync 而非阻塞处理。
- **Bounded subscriber 溢出导致额外 snapshot** → 以正确性优先；记录 overflow/resync 指标，并通过合理 channel 大小与 delta 合并减少正常情况下的重建。
- **旧页面不理解新 schema** → WebSocket handshake 显式协商 schema；不兼容时返回 reload-required，不让客户端无限 reconnect。
- **Push subscription 文件包含可投递 endpoint 和 keys** → 使用用户配置目录、`0600`、原子写入，并避免在日志和 Fleet UI 中输出 endpoint 或 key。
- **Hub 重启后的首次升级不能唤醒从未重新登记的旧浏览器** → 这是从历史内存 store 无法恢复的限制；浏览器下一次打开后自动 reconciliation，此后的重启保持持久。
- **Fleet health 可能把暂时延迟误报为 stale** → 显示原始 last seen/sync age，stale threshold 可配置，并把 reason code 与 hard incompatibility 区分。
- **自动 rollback 本身可能失败** → Backup 与 journal 在 rollback 验证成功前绝不删除，错误同时报告新旧版本状态并保留手动 recovery 路径。
- **Pinned Sigstore trusted root 轮换后旧 updater 可能拒绝合法新 release** → Root snapshot 与 identity policy 版本化并通过 fixture 验证轮换；失败时保持 fail closed，由受审查的新 binary 或手动验证安装路径完成 trust-root 更新，绝不降级为仅信 checksum。
- **本地字体增加 binary 体积** → 仅打包实际使用的 WOFF2/subset、lazy-load 非首屏字体，并由压缩后预算约束。
- **移动 toolbar 占用 terminal 空间** → 允许收起，遵守 safe-area，并仅在触屏或用户启用时默认展示。
- **WebKit 无法完整自动化标准 Push** → 把 Push 协议/Service Worker 行为放在 Chromium 与 worker contract 测试，WebKit 聚焦安装 guidance、layout、toolbar、clipboard error 与 accessibility。
- **纯 Hub 不再直接看见服务器本机 tmux** → 这是隔离边界；需要本机 session 时，在宿主安装同版本 Agent 并通过可信 Hub URL 配对。
- **现有代码假设 `tmux.Client` 总存在** → 用显式 Hub core/local integration interfaces 重构并进行 role matrix 测试，不用散落 nil checks 掩盖错误。
- **Volume UID/权限错误会阻止 non-root image 写入** → Compose 创建受控 named volume，启动 preflight/doctor 报告路径、UID 与修复方式，不回退到临时 root filesystem。
- **File store 无法支撑多个 Hub replica** → Compose 和文档固定单实例；横向扩展需要未来独立的协调/存储设计，不能通过 `--scale` 偷渡。
- **容器重建使 7 天 browser Session 提前失效** → 明确 Session 是 process-scoped，保留 Passkey 并要求重新登录；跨重启 Session durability 需另行进行安全设计。
- **Mutable image tag 使回滚目标不确定** → 发布 immutable SemVer/digest，生产及 rollback 示例固定 tag/digest，并保留 volume backup。

## Migration Plan

1. 引入 canonical coordinator、instance/revision envelope 和原子 WebSocket subscribe，同时保留现有读取 API 作为迁移桥梁。
2. 将 React state consumers 迁移到统一 provider/reducer，加入 connection generation guard、rehydrate 与 schema reload；确认不再由独立轮询覆盖权威状态。
3. 增加 durable Push store并从空 store 启动，部署后由已打开/再次打开的浏览器自动 reconciliation；更新 Service Worker payload 和 preference-aware sender。
4. 增加 health facts、Fleet UI 和 updater outcome 记录，不改变 Peer lifecycle 或增加远程执行。
5. 先集成 Go 内部 Sigstore verifier、pinned trusted root/identity policy 与 provenance fixtures，使 bundle 缺失或无效时在 staging 前 fail closed。
6. 给 pure Hub、standalone 和 Agent Unix listener 增加 health response，实现 staged update、journal、自动/manual rollback，并先通过 fake service 故障注入测试。
7. 添加移动 toolbar、safe-area/Visual Viewport 布局和本地字体/Nerd symbols；移除第三方字体引用并建立 bundle budgets。
8. 提取 Hub core 与 local integration，新增纯 `hub` 和显式 `standalone` 组合、保留 `server` 兼容入口，并以 role matrix/remote-only tests 证明纯 Hub 不调用 tmux。
9. 增加 XDG config/data、non-root OCI image、单实例 Compose、container-aware update/doctor/Fleet 和 GHCR release pipeline。
10. 运行 Go race/unit、frontend Hook、Chromium/WebKit Playwright、container/Agent/persistence、accessibility、外部请求和 bundle budget suites，再通过正常 release 流程部署。

回滚到旧 release 时，旧版本会忽略新增的 state runtime schema、Push subscription 和 updater metadata，也会回到仅校验 checksum 的旧信任模型；首次获得 provenance-aware binary 应通过现有可信发布或人工 Sigstore 验证路径完成。持久 Push 文件与 last-known-good backup 保留但不参与旧版本运行。由于 Service Worker 仍无 application cache，旧前端不会被新版本 shell 遮蔽。若新 updater 已留下未提交 transaction，应优先使用新版本的 recovery/rollback 命令完成恢复，再安装旧 binary。后续 trusted-root rotation 通过受审查的新 release 更新 pinned snapshot；旧 updater 拒绝新 root 时保持 fail closed。

原生 standalone/server 部署升级后保持原 command 语义。迁移到 Docker 时先停止原服务、完整备份并复制 config/data、校正 non-root ownership、保持相同 `TMUXATLAS_PUBLIC_URL`，再启动纯 Hub container；原服务器的 tmux 通过宿主 Agent 重新接入。Docker 回滚保留 volume 并切换 previous image tag/digest，不得删除 volume。旧版本若没有 `hub` command，不能用旧 image 继续解释新 Compose，必须按发布说明选择具备纯 Hub role 的最早兼容版本。

## Open Questions

None. Fleet Health 仅提供事实、分类和复制命令；远程执行或 Fleet 批量升级需要独立 capability 和单独的授权设计。Docker deployment 固定单 Hub、一个持久 volume、纯 Hub image 和 process-scoped browser Session；Cloudflare Containers、外部数据库与横向扩展不在本 change 中保留待选分支。

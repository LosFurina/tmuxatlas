## Why

TmuxAtlas 已具备实时 session 展示、多主机聚合、Web Push、PWA 和自更新能力，但当前浏览器状态由初始 HTTP 请求、周期轮询和 WebSocket 增量分别维护，没有统一 revision 或单写入者约束。客户端断线期间丢失事件后不会完整 rehydrate，旧 WebSocket 的 `onclose` 也可能在组件卸载或新连接建立后继续触发 ghost reconnect，导致状态过期、重复连接和重复处理。

运维闭环也不完整：Push subscription 仅保存在内存中，Hub 重启后无法继续推送；通知缺少稳定的 host 身份且偏好未在投递路径中一致执行；UI 无法识别 Fleet 版本漂移或健康异常；Updater 替换二进制并重启服务后没有验证新版本健康，也不能自动恢复上一版本。当前 `server` 还把 Hub 的 Web/Peer 职责与本机 tmux discovery、进程扫描和 PTY 执行强耦合，导致服务器无法以不挂载宿主机 tmux socket、不共享 PID namespace 的普通容器安全运行。PWA 已让移动端成为正式入口，但 Terminal 仍缺少触屏工具栏、safe-area 适配和完整的移动端验证，Web UI 还依赖运行时第三方字体资源。

本 change 将把这些能力收敛为可排序、可恢复、可观测和自包含的运行模型，使单 Hub 与多主机部署在断线、重启、升级和移动访问场景下保持一致行为。

## What Changes

- 建立 Hub-owned、revisioned、single-writer 的浏览器状态投影：
  - 所有面向 Web UI 的 session、host、tool event、activity 和 health 变更通过单一提交路径写入。
  - 每次原子提交产生单调递增 revision，并与 Hub instance identity 一起标识状态世代。
  - 为客户端提供带当前 revision 的完整 snapshot，以及严格有序的 delta；客户端发现 revision gap、instance 变化或无法应用的 delta 时强制重新获取 snapshot。
  - 前端以统一 state provider/reducer 消费 snapshot 与 delta，避免多个 Hook 的重叠请求以旧响应覆盖新状态。
- 重构浏览器状态 WebSocket 生命周期：
  - 每个挂载实例至多拥有一个活动连接和一个重连计时器。
  - 使用 connection generation/disposed guard 阻止旧 socket、StrictMode cleanup、`pageshow` 和过期 timer 创建 ghost connection。
  - 断线采用有上限的 exponential backoff 与 jitter；重新连接后先完成 state rehydrate，再把 UI 标记为 ready/connected。
  - 组件卸载、登出和目标切换必须取消 timer、注销 handler 并关闭对应连接。
- 扩展 Web Push 与现有 PWA 能力：
  - 将 Push subscriptions 原子持久化到 TmuxAtlas 配置目录，并在 Hub 重启后恢复；失效 endpoint 继续按服务响应安全清理。
  - 浏览器启动时核对 Service Worker、浏览器 subscription 与 Hub 持久状态，只有服务器确认登记后才显示 `subscribed`。
  - Push payload、notification tag 和点击目标携带稳定的 host ID、host display name、session、window/pane 与 status，远端通知必须导航到对应主机。
  - 服务端投递实时读取并执行 `preferences.notifications.statuses`；Waiting、Error、Completed 的页面通知与 Push 行为保持一致。
  - 保持现有 network-only PWA 语义，不增加离线 shell、API cache、终端 cache 或 mutation queue。
- 新增 Fleet Health：
  - 以稳定 host ID 展示本机和各 Agent 的在线状态、last seen、当前版本、Hub 版本、版本漂移、Agent/hook 配置健康和最近状态同步时间。
  - 对 unknown、healthy、stale、offline、version-behind、version-ahead/incompatible 等状态给出明确但非破坏性的分类和 remediation。
  - 为每台机器提供适用的检查或更新命令，展示最近一次本地 updater 结果。
  - Fleet Health 是观测与操作指引能力；本 change 不增加浏览器远程执行任意命令。
- 使 Self-updater 具备事务式恢复能力：
  - 下载目标 release 的 `checksums.txt` 与 `checksums.txt.sigstore.json`，先用 Go 内部 verifier 和内置 trusted root 验证 checksum bundle；身份策略固定 GitHub Actions OIDC issuer、`LosFurina/tmuxatlas` repository、`.github/workflows/goreleaser.yml` workflow 与目标 tag，bundle 缺失或任一验证失败时 fail closed。
  - 下载、checksum 校验和解包完成后先 staging 新二进制，并在替换前保留同文件系统上的上一版本。
  - 重启 systemd/launchd 服务后，在有界超时内验证服务处于运行状态、报告目标版本，并通过适合 pure Hub、standalone 或 Agent 角色的 health check。
  - 新版本未启动、版本不匹配或 health check 失败时，自动恢复上一二进制、再次启动服务并报告 rollback 结果。
  - 成功健康检查后才提交更新并清理临时状态；中断留下的 update transaction 可在下次运行时诊断和安全恢复。
  - 增加显式检查与手动 rollback 路径，并为 replace、restart、health failure、rollback success/failure 建立确定的退出状态和测试夹具。
- 拆分可独立部署的纯 Hub runtime：
  - 新增不初始化 tmux client、discovery、control mode、Agent detector、silence monitor 或本机 PTY executor 的 `tmuxatlas hub` 角色，仅承载 Web、Passkey、Peer registry、状态聚合、Push 和远程 PTY relay。
  - 保持 `tmuxatlas agent` 为宿主机 outbound-only 服务；新增显式 standalone 组合角色，并让旧 `tmuxatlas server` 保留为兼容入口。
  - Hub-only 模式不得把缺失或远端 target 回退到容器本机；没有在线 Agent 时仍应健康启动并提供管理与配对界面。
  - systemd/launchd installer、`doctor`、health probe 和日志必须识别 `hub`、`agent` 与 standalone，而不是用“是否存在 tmux”推断角色。
- 提供服务器优先的官方 Docker 部署：
  - 增加 multi-stage、非 root、包含系统 CA 且不安装 tmux 的 Hub OCI image，以及单实例 `compose.yaml`、`.dockerignore`、部署环境示例和健康检查。
  - Compose 默认只把 origin 映射到宿主机 `127.0.0.1`，由 Cloudflare Tunnel 或 Nginx+ACME 提供可信 HTTPS；不在应用或容器中恢复自签名 TLS。
  - 使用一个持久 volume 保存 Passkey、Hub identity、Peer trust、preferences、VAPID 与 Push subscription；不引入 PostgreSQL、Redis 或其他外部状态服务，并明确只支持单 Hub writer。
  - 容器不得要求 privileged、host PID/network、Docker socket 或宿主机 tmux socket；使用只读 root filesystem、临时目录、drop capabilities 和 `no-new-privileges` 等安全默认值。
  - Tag release 自动发布 `linux/amd64` 与 `linux/arm64` GHCR image、immutable digest、SBOM、provenance 和 keyless signature；CI 必须执行镜像构建、启动、health、Agent 配对/远程终端及 volume 重建验证。
  - 容器内 `tmuxatlas update` 不得原地替换镜像中的 binary，而应给出 `docker compose pull`/`up -d` 和固定旧 image tag rollback 指引。
- 新增移动端 Terminal 体验：
  - 提供适合触屏的可收起工具栏，至少覆盖 Esc、Tab、Ctrl、Alt、方向键、Copy、Paste 和软键盘控制。
  - Modifier 支持单次锁定或组合输入，并清楚显示当前状态，不能与 tmux/xterm 的原始控制字符语义冲突。
  - Top bar、Terminal、toolbar、modal 和 status 区域适配 `env(safe-area-inset-*)`、横竖屏、窄屏 sidebar/drawer 与软键盘 viewport 变化。
  - 保留桌面键盘直通和 xterm selection/scrollback 行为，移动端辅助控件必须具有可访问名称、足够触控目标和键盘等价操作。
- 使 Web 字体与符号资源自包含：
  - 移除运行时 Google Fonts 请求，将实际使用的 UI/Terminal 字体以本地 WOFF2 和明确许可证随前端构建。
  - 提供本地 Nerd Symbols 或经过记录的最小 glyph subset，并把它作为 Terminal/UI 的 fallback，而不要求用户系统预装 patched Nerd Font。
  - 区分 bundled font 与 system font；设置中不得提供实际无法渲染的字体选项。
  - 仅 preload 首屏必需字体，其余字体按使用加载，并为字体、xterm chunk 和首屏资源建立 bundle budget 与无外部字体请求测试。
- 补齐自动化验证：
  - Go 单元与并发测试覆盖 single-writer、revision 顺序、snapshot、gap recovery、Push persistence/preferences、host-aware payload 和 updater rollback。
  - React/Hook 测试覆盖 stale response、StrictMode mount/unmount、单连接约束、timer cleanup 和 reconnect rehydrate。
  - Playwright 覆盖同名 session 的多主机身份、真实应用 Service Worker 注册、远端 Push 跳转、Hub 重启后的 subscription 恢复、版本更新提示和断线恢复。
  - 增加 Chromium 与 WebKit 的窄屏、safe-area、触摸工具栏、Clipboard 和可访问性检查，并对测试中的外部字体请求和 bundle regression 直接失败。

## Capabilities

### New Capabilities

- `revisioned-state-sync`: Hub 单写入者状态投影、instance/revision、完整 snapshot、有序 delta、客户端 gap recovery，以及无 ghost reconnect 的浏览器状态 WebSocket 生命周期。
- `fleet-health`: 基于稳定 host identity 的版本漂移、在线/陈旧状态、Agent 配置健康、状态同步新鲜度和 remediation 展示。
- `reliable-self-update`: 通过 pinned Sigstore trusted root 与 GitHub Actions release identity 验证 checksum provenance，并提供 staged binary replacement、服务重启、模式化 health check、自动与手动 rollback、事务恢复、deployment-aware Docker 拒绝/指引和可验证退出状态。
- `mobile-terminal`: 触屏 Terminal modifier/clipboard 工具栏、软键盘交互、safe-area、窄屏布局和移动端可访问性。
- `self-hosted-web-assets`: 本地字体与 Nerd symbols、字体可用性契约、加载策略、许可证记录、外部请求禁用和 bundle budgets。
- `hub-runtime`: 不依赖本机 tmux 的纯 Hub 角色、显式 runtime composition、role-aware health/doctor/service 安装，以及与 standalone 兼容入口的边界。
- `docker-deployment`: 普通服务器上的单实例 OCI/Compose Hub 部署、持久 volume、安全默认值、可信反向代理、镜像发布与升级/回滚契约。

### Modified Capabilities

- `progressive-web-app`: 将现有 Service Worker 与 Push 要求扩展为 subscription 持久化和启动时 reconciliation，并要求 host-aware、安全同源的通知目标及服务端通知偏好一致性；继续保持 network-only、无离线缓存的应用行为。

## Impact

- `pkg/state`、`pkg/ws`、`pkg/server` 及相关状态生产者将接入单写入者 projection、revisioned snapshot/delta 和 reconnect rehydrate 契约。
- `web/src/App.tsx`、现有 state/WebSocket/Terminal/notification Hooks 和主要导航组件将迁移到统一状态 provider，并采用稳定的 `host/session` identity。
- `pkg/webpush`、`pkg/preferences`、Service Worker 和 Settings/Setup 通知 UI 将支持持久 subscription、host-aware payload 和端到端偏好执行。
- `pkg/commands/update`、systemd/launchd service handling、版本/健康端点及 CLI 输出将增加 Go 内部 Sigstore bundle 验证、pinned trusted root/issuer/repository/workflow/tag policy、staging、health check、transaction metadata 和 rollback；release checksum 只有在 provenance 成功后才会被信任。
- `pkg/commands/server`、`pkg/server`、`pkg/commands/install`、`pkg/commands/doctor` 和启动 wiring 将拆分纯 Hub 与 standalone 组合，使 Hub 在 `tmux.Client`、本机 state manager 和本机 PTY executor 均不存在时保持安全。
- 仓库将新增 `Dockerfile`、`.dockerignore`、`compose.yaml`、Docker 环境示例及部署文档；release workflow 将发布并签名 GHCR multi-arch image。
- Host/Fleet 页面将消费现有 Host version/online 数据，并增加健康投影；可能扩展既有状态 payload，但不改变 Peer authentication、pairing、heartbeat、reconnect 或连接管理。
- `web/public`、CSS、Vite 构建和发布产物将新增本地字体、Nerd symbol 资源及许可证文件；二进制体积可能小幅增加，但运行时不再依赖第三方字体服务。
- 配置目录将新增 Push subscription 和 updater transaction/backup 元数据。缺少这些文件的现有安装按空状态启动，不需要破坏性迁移。
- 浏览器内部状态 WebSocket 消息将增加 instance/revision/snapshot 信息；同一发布内的 Hub 与嵌入前端同步升级，旧页面连接新 Hub 时应通过 schema/version 检测触发 reload 或完整 rehydrate。
- CI 将增加状态并发、Sigstore valid/invalid provenance fixtures、更新失败注入、WebSocket lifecycle、Push restart、多主机、WebKit/mobile、accessibility、外部请求和 bundle budget 覆盖。
- 本变更不在 TmuxAtlas 内终止 TLS，不引入 Cloudflare Containers、Kubernetes、PostgreSQL、Redis、横向多 Hub writer、离线终端、远程任意命令执行或自动化 Fleet 批量升级。Docker 仍沿用可信网关与现有 Passkey/Session 安全模型；浏览器 Session 是进程内状态，容器重建后需要重新使用 Passkey 登录，即使配置的 idle TTL 尚未到期。

## 1. Canonical State 契约

- [x] 1.1 定义 state `schema_version`、`instance_id`、snapshot/delta envelope、typed operation 和 stable host/session/window/pane key，并用 table tests 验证序列化与跨主机同名 session 不冲突。
- [x] 1.2 实现单 goroutine state coordinator 与 typed mutation 提交入口，确保 material commit 单调增加 revision、no-op 不增 revision，并添加并发 producer 单元测试。
- [x] 1.3 实现 immutable/defensive snapshot 与原子 subscribe，使用 bounded subscriber queue 在 overflow 时返回 resync outcome，并测试 snapshot 与首个 delta 无漏事件窗口。
- [x] 1.4 实现 Hub process `instance_id`、state schema compatibility 检查和 restart 世代语义，并测试新 instance 不与旧 revision 混用。

## 2. 后端 State 集成与 WebSocket

- [x] 2.1 将本机 discovery 与现有 host/session 聚合接入 coordinator，统一用 stable identity 生成 projection，并验证单机及两个 host 同名 session 的 snapshot。
- [x] 2.2 将 tool event、activity、preferences/updater health facts 接入同一 mutation 流，确保一次逻辑更新产生有序 delta，并添加 producer adapter 测试。
- [x] 2.3 重构浏览器状态 WebSocket，使其协商 schema、先发送原子 snapshot、再发送 ordered delta，并覆盖 resync-required、slow subscriber 和 reload-required handler tests。
- [x] 2.4 让迁移期 `/api/sessions`、`/api/hosts`、tool/activity/health 读取从 canonical projection 派生，运行相关 Go tests 与 `go test -race` 验证无数据竞争。

## 3. 前端 Reducer 与连接生命周期

- [x] 3.1 配置 Vitest、jsdom 与 React Testing Library，加入 `test` npm script、StrictMode render helper、WebSocket/visibility/pageshow mock 和 fake-timer 基础设施，并以 reducer 与 Hook smoke tests 验证本地及 CI runner 可执行。
- [x] 3.2 实现 normalized application state reducer，支持 snapshot replace、ordered delta、duplicate ignore、gap/instance mismatch rehydrate，并添加 TypeScript unit tests。
- [x] 3.3 添加 application state provider 和 session/host selectors，迁移 Sidebar、Overview、StatusBar 与路由目标到 stable `host/session` identity，并验证同名 session 切换不会复用错误 Terminal target。
- [x] 3.4 迁移 tool event、activity、health consumers 与 session mutation pending 状态，移除可覆盖权威 revision 的独立轮询副本，并用 stale-response tests 验证旧 HTTP 响应无效。
- [x] 3.5 实现带 generation/disposed guard 的 browser connection controller、单 socket/单 timer 约束及 capped exponential backoff+jitter，并用 StrictMode、fake timer、visibility/pageshow Hook tests 验证无 ghost reconnect。
- [x] 3.6 接入 `connecting`、`rehydrating`、`ready`、`reconnecting`、`auth-required` 和 reload-required UI，验证 reconnect 只有在 snapshot 应用后才显示 ready。

## 4. Push 与 PWA 生命周期

- [x] 4.1 实现 `0600`、同目录临时文件加 rename 的 durable Push subscription store，覆盖 startup reload、endpoint dedupe、unsubscribe、404/410 expiry 和损坏写入不覆盖旧文件。
- [x] 4.2 扩展 Push sender，使 payload 携带 stable host/session/window/pane/tool/status、tag 跨 host 唯一，并实时执行 Waiting/Error/Completed preferences；添加过滤与同名远端 session tests。
- [x] 4.3 重构 Service Worker/Push Hook 启动 reconciliation，只有浏览器 subscription 与 Hub 持久化均成功才报告 `subscribed`，并用 unit tests 覆盖 server failure、retry 与 unsubscribe 部分失败。
- [x] 4.4 更新 Service Worker 的 host-aware 同源导航和 malformed fallback，扩展 PWA tests 验证真实应用注册路径、network-only 行为及不同 host 通知不碰撞。

## 5. Fleet Health

- [x] 5.1 定义 host health facts、freshness threshold 和可组合 reason codes，实现 unknown、stale、offline、version-behind/ahead、explicit incompatible、healthy 分类并添加 table tests。
- [x] 5.2 通过现有 state/stats 数据路径汇集 role、version/commit、last sync、Agent/hook checks 与最近 updater outcome，不改 Peer 连接生命周期，并验证缺失 metadata 保持 unknown。
- [x] 5.3 实现 Fleet Health UI，以 stable host ID 展示摘要、全部 reason、原始证据和可复制 remediation，确保页面不执行命令且同名 host 保持独立。
- [x] 5.4 添加多 host browser/component tests，覆盖版本漂移、stale/offline、incompatible、rolled-back updater、duplicate display name 和 reconnect 后同 revision 刷新。

## 6. Updater Transaction 与 Rollback

- [x] 6.1 实现 Go 内部 Sigstore checksum-bundle verifier、版本化 pinned trusted root，以及固定 issuer、`LosFurina/tmuxatlas` repository、`.github/workflows/goreleaser.yml` workflow 和目标 tag 的 identity policy；用 checked-in fixtures 覆盖 valid、invalid issuer/repository/workflow/tag、missing/malformed bundle 与 root rotation，并验证全部失败在 staging 前 fail closed。
- [x] 6.2 将 updater 拆为 verified staging、last-known-good backup 与 atomic replacement 阶段，添加 provenance/checksum/archive/permission/replace failure tests 验证当前 executable 不被破坏。
- [x] 6.3 实现 durable update transaction journal、previous release metadata 和中断 recovery，覆盖 staged/replaced/restarted/healthy/rolling-back/rolled-back 状态转换测试。
- [x] 6.4 为 pure Hub、standalone 与 Agent 的本地 Unix HTTP listener 增加 role/deployment/version/commit/instance/ready health response，并测试三种角色在真正 ready 前不会误报成功。
- [x] 6.5 扩展 systemd/launchd service discovery 与 updater health loop，验证 active、role、target version 和 bounded readiness timeout 后才 commit，并使用 fake service/probe 测试。
- [x] 6.6 实现自动 rollback、显式 rollback/recovery、`--no-restart` installed/running version 区分和 recent outcome 持久化，完成 restart failure、timeout、version mismatch、rollback success/failure 故障注入矩阵。

## 7. 移动端 Terminal

- [x] 7.1 实现可收起 touch Terminal toolbar 与 Esc、Tab、方向键、Ctrl/Alt one-shot/locked sequence 编码，添加输入单元测试和 target change modifier reset 测试。
- [x] 7.2 实现 generation-safe Copy/Paste 与软键盘控制，覆盖 selection、permission denied、empty clipboard、API unavailable 和 clipboard Promise 期间切换 session。
- [x] 7.3 将窄屏 Sidebar 改为可关闭 drawer，使 TopBar、Terminal、toolbar、modal、StatusBar 使用 safe-area inset 和 viewport-relative 尺寸，并用组件 viewport tests 验证横竖屏。
- [x] 7.4 接入 Visual Viewport 软键盘变化及 coalesced xterm fit/PTY resize，为所有 toolbar control 增加 accessible name/state 和 44×44 触控目标，并验证桌面键盘/selection/scrollback 无回归。

## 8. Self-host Fonts 与 Bundle

- [x] 8.1 移除 Google Fonts CSS/preconnect，vendor 实际使用的 WOFF2 字体及 license/source metadata，修正 bundled/system/generic font 设置映射并测试每个 bundled option 可加载。
- [x] 8.2 加入本地 Nerd Symbols Only 或可复现最小 subset、fallback font-family 和 glyph/license inventory tests，验证支持 glyph 不依赖系统 patched font。
- [x] 8.3 按 Login/Terminal 使用路径 lazy-load xterm 与非首屏字体，建立 version-controlled gzip/Brotli bundle budgets 和第三方字体请求拦截测试。

## 9. 浏览器测试与 CI

- [ ] 9.1 建立隔离 Hub 与多 host Playwright fixture，覆盖跨 host 同名 session、断线期间 mutation、reconnect snapshot rehydrate、revision gap 和旧页面 reload-required。
- [ ] 9.2 扩展 Chromium PWA/Push E2E，覆盖真实应用 Service Worker 注册、Hub restart subscription 恢复、preferences、Completed、host-aware click、expiry 和无 application cache。
- [ ] 9.3 增加 mobile WebKit project 与 accessibility checks，覆盖 portrait/landscape、safe-area、drawer/modal、toolbar sequence、clipboard failure、soft keyboard resize、focus/ARIA 和触控目标。
- [ ] 9.4 更新 CI 分阶段运行 frontend type/unit/Hook tests、`go test -race ./...`、Chromium/WebKit Playwright、accessibility、external-request 和 bundle-budget gates，并保留失败 trace/artifact。

## 10. 纯 Hub Runtime 与角色组合

- [ ] 10.1 定义 `hub`、`agent`、`standalone` 的显式 role/dependency graph 和共享 Hub core interfaces，保留 `server` 作为 standalone 兼容入口，并用构造测试证明纯 Hub 不调用 tmux lookup、discovery、control mode、process detector、silence monitor、本机 activity/stats 或本机 PTY executor。
- [ ] 10.2 从当前 `server` wiring 提取 Hub core 与可选 local integration，实现 `tmuxatlas hub` 和 `tmuxatlas standalone`，让现有 `server` 行为保持兼容，并覆盖无 tmux binary、无 Passkey、无 Agent 和全部 Agent offline 时仍可启动。
- [ ] 10.3 让 `pkg/server`、canonical projection、Router、HTTP/WS handlers 和 remote PTY relay 在无 local manager/client/executor 时安全工作；纯 Hub 只接受已连接 Agent target，对 Hub 本机或缺失 target 返回 structured error 且绝不 fallback。
- [ ] 10.4 扩展安装脚本、`tmuxatlas install --mode hub`、systemd/launchd、`doctor`、Fleet facts 和 Unix health command，使 role/deployment mode 显式可见，并验证 setup-required 与无在线 Agent 不会误判 process readiness。
- [ ] 10.5 实现 Hub/standalone 有界 graceful shutdown，按顺序停止新请求、关闭 browser/Peer/PTY、Push/coordinator 和 Unix socket；用 active WebSocket、重复取消和 SIGTERM tests 验证无 panic、deadlock、goroutine/socket 泄漏。

## 11. Docker 镜像、Compose 与发布

- [ ] 11.1 让 `paths.ConfigDir` 遵循 `XDG_CONFIG_HOME`，统一 `/var/lib/tmuxatlas/config` 与 `/var/lib/tmuxatlas/data` 的 non-root 权限和原子文件语义，并测试 volume 中 Passkey、identity、Peer trust、preferences、VAPID 与 Push store 的目录归属。
- [ ] 11.2 新增 `.dockerignore` 与 multi-stage `Dockerfile`，构建嵌入 Web 的静态 binary 和仅含系统 CA/non-root 用户的纯 Hub runtime image；验证 image 不含 tmux、shell、编译器或 package manager，并支持 `linux/amd64`、`linux/arm64`。
- [ ] 11.3 新增单服务 `compose.yaml` 与 Docker 环境示例，强制 `TMUXATLAS_PUBLIC_URL`，示例 `TMUXATLAS_SESSION_TTL=168h`，配置 container `0.0.0.0:7654`、host `127.0.0.1:7654`、单 named volume、read-only root、tmpfs、cap drop、`no-new-privileges`、restart、healthcheck 与 stop grace period，且不包含 DB/Redis、privileged、host PID/network 或敏感 socket mount。
- [ ] 11.4 实现 container-aware `update --check`、install/rollback fail-closed、`doctor` 与 Fleet remediation；官方 Docker 模式不得替换 binary 或调用 systemd/launchd，并输出 Compose pull/recreate、health 和 previous image tag/digest rollback 命令。
- [ ] 11.5 扩展 tag release workflow，以最小 `GITHUB_TOKEN packages:write` 发布不可覆盖的 GHCR SemVer multi-arch manifest、OCI version/revision labels、SBOM、provenance 与 keyless signature；PR CI 只 build/inspect/smoke test且不得 push。
- [ ] 11.6 增加 container integration suite，覆盖 `docker compose config`、无 tmux启动、non-root/read-only/capability 默认值、Unix health、短期 bootstrap、真实 tmux Agent pairing/state/PTY/input/resize、SIGTERM 和 Agent 自动重连。
- [ ] 11.7 在保留 volume 的 recreate 测试中验证 Passkey、Hub fingerprint、Peer trust、preferences、VAPID 与 Push subscription 不变，旧 browser Session 明确失效且需 Passkey 重登，并确认整个 supported topology 不启动外部数据库/cache。

## 12. 文档与最终验证

- [ ] 12.1 编写 canonical state schema、single-writer、snapshot/delta、reconnect/reload 和 stable identity 的开发文档，并记录 Peer lifecycle 与 terminal stream 不在该模型内。
- [ ] 12.2 更新 README/原生运维文档，说明 Fleet Health、Push restart reconciliation、updater health/rollback/recovery、移动 toolbar/safe-area、bundled/system fonts、纯 Hub/standalone/Agent 角色和已知迁移行为。
- [ ] 12.3 编写服务器 Docker 部署文档，覆盖可信 Nginx+ACME/Cloudflare Tunnel 网关、Public URL、bootstrap/pair 的 `docker compose exec`、宿主 tmux 另装 Agent、volume backup/restore、native-to-Docker 迁移、image upgrade/health/rollback、单副本限制和禁止 `down -v`。
- [ ] 12.4 完成生产前验证：运行 frontend build、全部 unit/Hook/E2E、`go test -race ./...`、`go vet ./...`、bundle/external-request checks、Docker build/Compose/container/persistence/release metadata gates、`openspec validate improve-state-web-and-operations --strict` 与 repository whitespace/status 检查并修复所有失败。

## 1. 纯 Hub Runtime

- [x] 1.1 重构 Peer manager，使其支持无 local tmux manager 的 remote-only Hub，并加入“不注册本地主机、不采集本机 stats、不执行本机 target”的测试。
- [x] 1.2 从 `server` 命令抽取共享 Hub core 构造，新增不调用 `tmux.NewClient` 的 `tmuxatlas hub`，保留 `server` standalone 兼容入口。
- [x] 1.3 让 HTTP API、state WebSocket、Session mutation 与 PTY routing 在无 local Client/Detector/Activity producer 时安全工作，并测试 active/offline/missing Agent target。
- [x] 1.4 为 Hub/standalone 增加准确的 role/deployment health、context-aware Peer lifecycle 与有界幂等 shutdown 测试。

## 2. 路径、诊断与容器生命周期

- [x] 2.1 让 `paths.ConfigDir` 遵循 `XDG_CONFIG_HOME`，测试 config/data/runtime 的单卷 XDG 布局和 legacy migration。
- [x] 2.2 新增通过私有 Unix socket验证 role、deployment、version、commit 与 ready 的 `tmuxatlas healthcheck` 命令及测试。
- [x] 2.3 让 `install`、`update`、rollback/recover、doctor 与 Fleet remediation 识别 `TMUXATLAS_DEPLOYMENT=docker`；所有 mutation 在下载、临时写入或 service manager 操作前 fail closed。
- [x] 2.4 扩展 `tmuxatlas install --mode hub` 和原生 service health，使纯 Hub 也可在无 tmux 的服务器上以 systemd/launchd运行。

## 3. 官方镜像与 Compose

- [x] 3.1 新增 `.dockerignore` 与 multi-stage `Dockerfile`，构建嵌入 Web 的静态 binary 和 scratch non-root runtime，复制 CA/minimal identity，并预置 UID/GID 65532 的持久目录。
- [x] 3.2 新增 `compose.yaml` 与 `.env.docker.example`，强制 Public URL，配置单卷、loopback publication、read-only root、tmpfs、cap drop、no-new-privileges、healthcheck、restart 与 stop grace period。
- [x] 3.3 添加 image/Compose 静态检查，验证无 shell、tmux、compiler/package manager，容器 user、entrypoint、labels、端口和安全选项正确。
- [x] 3.4 添加容器 smoke/integration test，覆盖无 tmux启动、Passkey bootstrap、Agent pairing、remote state/PTY、SIGTERM、同卷 recreate 与 durable identity。

## 4. CI、GHCR 与供应链

- [x] 4.1 在 PR/main CI 中加入 Buildx cache、候选镜像 build、security inspection、Compose render 和 container smoke gate，失败时上传诊断。
- [x] 4.2 扩展 Tag Release，以最小 `packages:write`/attestation 权限发布 GHCR linux/amd64 与 linux/arm64 manifest、SemVer labels、SBOM 和 provenance。
- [x] 4.3 使用 keyless Cosign 签名 image digest，并确保只有所有 binary/browser/container gates 成功后才更新 version/minor/latest tags。

## 5. 文档、迁移与最终验证

- [x] 5.1 更新 README 与 Docker 部署文档，覆盖 Cloudflare Tunnel/Nginx、bootstrap/pair、宿主 Agent、备份恢复、native-to-Docker migration、升级、health、digest rollback、single-writer 和禁止 `down -v`。
- [x] 5.2 运行 frontend unit/build/bundle checks、`go test -race ./...`、`go vet ./...`、Playwright、Docker/Compose/container tests 和 OpenSpec strict validation，修复全部失败。
- [x] 5.3 检查 diff、镜像 metadata、仓库状态与文档命令可复制性，提交变更但不自动切换现有生产 Hub。

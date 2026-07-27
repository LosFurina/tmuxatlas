## Context

当前 `tmuxatlas server` 在一个函数中同时构造 Hub、tmux discovery、control mode、detector、silence monitor、本机 PTY 与 Peer runtime。主规格已经要求纯 `hub` 角色和官方 Docker 镜像，但仓库仍只有 standalone `server`，且 `paths.ConfigDir` 不遵循 XDG、server router 对本机 tmux 依赖存在直接解引用。现有 Release 只发布原生压缩包，没有 OCI 制品。

容器部署仍以单服务器、单 Hub、外部可信 TLS 网关为目标。Passkey、Hub identity、Peer trust、preferences、VAPID 与 Push subscription 都是本地文件状态，必须放在一个持久卷中；浏览器 Session 是进程内状态，重建后允许失效。

## Goals / Non-Goals

**Goals:**

- 提供不安装或探测 tmux 的纯 Hub composition。
- 生成最小、non-root、read-only、multi-arch OCI image。
- 用单 named volume 和 XDG 子目录持久化全部 Hub durable state。
- 提供可直接用于 Cloudflare Tunnel/Nginx 的 loopback Compose 部署。
- 让 PR 与 Tag 流水线验证并发布可追溯的 GHCR 镜像。
- 保持原生 `server`、Agent、Passkey 与 Peer 协议兼容。

**Non-Goals:**

- 本单不建立 Testing/Production 自动晋级；它只提供该流水线依赖的不可变容器制品。
- 不支持多副本、多写者共享卷、Redis、数据库或 Kubernetes。
- 不把 Cloudflare Tunnel 或 Nginx 打包进应用容器。
- 不迁移生产数据或切换当前生产服务。

## Decisions

### 1. 显式拆分 Hub core 与 local integration

新增 `tmuxatlas hub`，通过共享构造函数初始化认证、偏好、Push、identity、Peer、pairing、canonical state 和 HTTP/WS。只有 standalone 路径注入 tmux client、state manager local producer、detector 与本机 PTY executor。Peer manager 支持没有 local manager 的 remote-only 模式，并且纯 Hub 不注册虚假的本地主机。

保留 `tmuxatlas server` 作为 standalone 兼容入口；后续可增加 `standalone` alias，但不改变已有部署语义。

### 2. scratch runtime 与内置 healthcheck

Dockerfile 使用 Node/Go builder 构建静态 binary，最终镜像使用 `scratch`，只复制 binary、CA bundle、最小 passwd/group 与预创建的可写目录，运行 UID/GID 65532。镜像内没有 shell、tmux、编译器或 package manager。

新增 `tmuxatlas healthcheck`，通过私有 Unix socket读取 `/health`，避免为了 Docker HEALTHCHECK 引入 curl/wget 或公开无认证健康接口。

### 3. 单卷 XDG 布局

`paths.ConfigDir` 改为使用 `os.UserConfigDir()`，与 `DataDir` 的 XDG 行为一致。Compose 将：

- `XDG_CONFIG_HOME=/var/lib/tmuxatlas/config`
- `XDG_DATA_HOME=/var/lib/tmuxatlas/data`
- `XDG_RUNTIME_DIR=/run/tmuxatlas`
- `HOME=/var/lib/tmuxatlas/home`

named volume 挂载 `/var/lib/tmuxatlas`，tmpfs 提供 `/tmp` 与 `/run/tmuxatlas`。镜像目录在创建 volume 时已属于 non-root UID；bind mount 用户需按文档设置权限。

### 4. 安全 Compose 默认值

Compose 只发布 `127.0.0.1:7654:7654`，容器内监听 `0.0.0.0:7654`，强制最终 `TMUXATLAS_PUBLIC_URL`，默认 7 天 Session TTL。启用 read-only root、all capabilities drop、no-new-privileges、restart、bounded stop grace period 与内置 healthcheck。TLS 继续由 Cloudflare Tunnel 或 Nginx+ACME 终止。

### 5. 容器更新只允许 image recreate

官方镜像设置 `TMUXATLAS_DEPLOYMENT=docker`。`update --check` 可查询版本，但安装、rollback、recover 与 service restart 在任何下载/写入前失败，并输出 Compose pull/up/recreate 与 previous tag/digest rollback 指引。`install` 在 Docker deployment 中同样拒绝创建 systemd/launchd。

### 6. PR build gate 与 Tag GHCR 发布

PR/main CI 使用 Buildx 构建本地 image，检查 user、entrypoint、无 tmux/shell、read-only 启动和 health，并运行 Compose smoke test。Tag Release 在已有 binary release 成功后发布：

- `ghcr.io/losfurina/tmuxatlas:<semver>`
- `ghcr.io/losfurina/tmuxatlas:<major>.<minor>`
- `ghcr.io/losfurina/tmuxatlas:latest`

使用 `GITHUB_TOKEN packages:write`、GitHub provenance attestation、SBOM 与 keyless Cosign，不需要 PAT。

## Risks / Trade-offs

- [纯 Hub refactor 触及现有 server router] → 增加 role-matrix tests，并持续运行原生 Go/浏览器 E2E。
- [scratch 无调试工具] → 提供本地 socket health/admin 命令和明确日志；调试使用外部工具容器。
- [named volume 初始 ownership 与 bind mount 不同] → 镜像预置 ownership，文档明确 bind mount 的 `chown 65532:65532`。
- [Hub 重建使浏览器 Session 失效] → 明确这是预期进程态行为，Passkey 与所有 durable identity 保持不变。
- [multi-arch/签名增加 Release 时间] → 与现有 Tag Release 并行或在 binary CI 后独立执行，失败不发布 latest。
- [同一卷多副本会破坏文件状态] → Compose 固定单副本，doctor/文档明确 single-writer。

## Migration Plan

1. 在 PR CI 完成纯 Hub、image 与 Compose 测试，不触碰当前生产服务。
2. 发布候选 Tag，确认 GHCR manifest、SBOM、provenance 和签名。
3. 在独立 Testing 域名和新 volume 启动容器，注册独立 Passkey、配对测试 Agent并执行 PTY 验收。
4. 备份生产 `~/.config/tmuxatlas` 与 data 目录，停止原生 Hub，将内容复制到 volume 对应 XDG 目录并校正 UID。
5. 用固定 SemVer/digest 启动 Production Compose，切换 Tunnel origin，验证 `/health`、登录、Peer reconnect、PTY、Push。
6. 失败时停止容器、恢复 Tunnel origin 和原生 systemd 服务；保留 volume，不执行 `down -v`。

## Open Questions

- Testing/Production 自动晋级、GitHub Environments 与服务器部署凭据将在后续独立变更中完成。

## Why

TmuxAtlas 已经声明纯 Hub 与 Docker 部署能力，但上一次变更在相关任务尚未实现时被归档，导致当前仓库没有 `tmuxatlas hub`、Dockerfile、Compose 或 GHCR 镜像。先补齐不可变、可验证的容器部署单元，才能安全建立 Testing/Production 自动晋级流水线。

## What Changes

- 实现不依赖 tmux 的纯 `tmuxatlas hub`，保留 `server` 的 standalone 兼容行为。
- 让配置与数据目录遵循 XDG，并为单持久卷提供稳定、可迁移的容器目录布局。
- 提供 non-root、read-only、无 shell/tmux/package manager 的 multi-stage OCI image。
- 提供单 Hub `compose.yaml`、安全默认值、健康检查和 Cloudflare Tunnel/Nginx 网关文档。
- 让 update、doctor、install 和本地管理命令识别 Docker deployment，禁止容器内自替换 binary。
- 在 PR CI 中构建和 smoke-test 镜像，在 SemVer Tag Release 中发布 GHCR multi-arch 镜像、SBOM、provenance 和签名。
- 增加容器启动、持久化、Agent 配对、远程 PTY、优雅退出和重建恢复测试。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `hub-runtime`: 落实纯 Hub 的构造边界、角色命令、安装诊断和优雅退出行为。
- `docker-deployment`: 明确官方镜像、Compose、持久化、本地管理、发布和集成验证的可执行验收边界。
- `reliable-self-update`: 完成 Docker deployment 检测以及容器内更新/回滚 fail-closed 行为。

## Impact

- Go runtime composition、CLI 命令、路径与 updater/doctor/install。
- 新增 Dockerfile、Compose、容器入口与集成测试。
- GitHub Actions CI、Release 权限与 GHCR 发布产物。
- README 和服务器部署、升级、回滚、数据迁移文档。
- 现有原生 `server`、Agent、Passkey 与可信网关协议保持兼容。

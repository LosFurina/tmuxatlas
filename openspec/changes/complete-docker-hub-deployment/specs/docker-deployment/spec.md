## ADDED Requirements

### Requirement: Reproducible container acceptance gate
项目 SHALL 提供自动化 container acceptance gate，使用与官方镜像相同的 Dockerfile 和 Compose 边界验证 image metadata、non-root/read-only 启动、health、持久化与远程 Agent 行为。未通过 gate 的 Tag MUST NOT 更新 GHCR stable tags。

#### Scenario: Inspect minimal runtime
- **WHEN** CI 构建候选镜像并检查 filesystem、user 和 process
- **THEN** image 以 UID/GID 65532 运行，只包含应用 binary、CA、identity metadata 与必要目录，且找不到 shell、tmux、compiler 或 package manager

#### Scenario: Start hardened Compose
- **WHEN** CI 使用临时 Public URL 和 named volume 启动 Compose
- **THEN** Hub 在 read-only root、cap-drop、no-new-privileges 和 loopback host publication下变为 healthy

#### Scenario: Preserve durable state
- **WHEN** CI 创建 Hub identity/Passkey/Peer trust 后使用同一 volume recreate 容器
- **THEN** durable state 与 Hub fingerprint 不变，旧进程 Session 被拒绝，Agent可自动重连

### Requirement: Immutable GHCR image identity
每个稳定 Tag SHALL 发布绑定 Git commit 的 multi-architecture GHCR manifest，并提供 OCI labels、SBOM、provenance 与 keyless signature。Deployment 文档 MUST 推荐生产固定 SemVer 或 digest，不得依赖可变 latest 做回滚。

#### Scenario: Publish stable Tag
- **WHEN** `vX.Y.Z` 通过 binary、browser 和 container gates
- **THEN** GHCR 的 `vX.Y.Z` manifest 同时包含 linux/amd64 与 linux/arm64，并可验证 revision label、attestation 和 signature

#### Scenario: Container gate fails
- **WHEN** image smoke、security inspection 或 integration test失败
- **THEN** Release workflow失败，且不得创建或移动 `latest` 与 minor stable tag

### Requirement: Single-volume migration contract
项目 SHALL 提供从原生目录迁移到 `/var/lib/tmuxatlas` XDG 布局的可验证步骤，迁移 MUST 在 Hub 停止时执行并保留源备份。

#### Scenario: Migrate native Hub
- **WHEN** operator 停止原生 Hub、备份源目录、复制 config/data 并设置 UID/GID 65532
- **THEN** 容器使用原 Hub identity、Passkeys、Peer trust、preferences、VAPID 与 Push subscriptions启动

#### Scenario: Roll back migration
- **WHEN** 容器验收失败
- **THEN** operator 可停止容器、恢复原网关 origin 并启动未被修改的原生服务，且 volume 与源备份仍保留

## ADDED Requirements

### Requirement: Early container mutation refusal
当 `TMUXATLAS_DEPLOYMENT=docker` 时，任何会修改 executable、transaction journal 或 service manager 的 update/install/rollback/recover 路径 MUST 在网络下载、临时文件创建或服务查询之前 fail closed。

#### Scenario: Run update in official container
- **WHEN** operator 在官方容器执行 `tmuxatlas update`
- **THEN** 命令在下载 release 之前失败，并输出固定 image tag/digest 的 Compose pull、recreate、health 与 rollback 指引

#### Scenario: Run read-only update check
- **WHEN** operator 在官方容器执行 `tmuxatlas update --check`
- **THEN** 命令只读取当前与最新版本，不写 executable 或 transaction，并输出 Docker-specific upgrade guidance

#### Scenario: Install a service in container
- **WHEN** operator 在官方容器执行 `tmuxatlas install`
- **THEN** 命令拒绝创建 systemd/launchd 配置，并指向外部 Compose service lifecycle

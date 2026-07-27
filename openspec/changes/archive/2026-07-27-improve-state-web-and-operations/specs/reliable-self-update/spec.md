## ADDED Requirements

### Requirement: Trusted release provenance
Self-updater SHALL 下载目标 release 的 `checksums.txt` 与 `checksums.txt.sigstore.json`，使用 Go 内部 verifier 和 pinned trusted root 验证 checksum bytes 的 Sigstore bundle。Verifier SHALL 要求 OIDC issuer 精确等于 `https://token.actions.githubusercontent.com`、repository 精确等于 `LosFurina/tmuxatlas`、workflow 精确等于 `.github/workflows/goreleaser.yml`，且 workflow identity/ref 精确绑定 updater 已选定的 `refs/tags/<target-tag>`。任何 bundle 缺失、密码学验证失败或 identity policy 不匹配 MUST fail closed，且不得使用未验证 checksum。

#### Scenario: Valid release provenance
- **WHEN** bundle 验证 checksum bytes 成功，且 trusted root、issuer、repository、workflow 和目标 tag 全部匹配
- **THEN** updater 才可以解析该 checksum 并继续 candidate preflight

#### Scenario: Invalid OIDC issuer
- **WHEN** bundle 的签名证书 issuer 不是 `https://token.actions.githubusercontent.com`
- **THEN** updater 在 staging 前以非零状态退出，并保持当前 executable 与服务不变

#### Scenario: Invalid repository identity
- **WHEN** bundle 表示的 repository 不是 `LosFurina/tmuxatlas`
- **THEN** updater 拒绝该 bundle，不得信任其中认证的 checksum

#### Scenario: Invalid workflow identity
- **WHEN** bundle 的 workflow 不是 `.github/workflows/goreleaser.yml`
- **THEN** updater 拒绝该 bundle，即使签名来自同一 GitHub organization

#### Scenario: Invalid target tag
- **WHEN** bundle workflow identity/ref 中的 tag 与 updater 已选定的 target tag 不同
- **THEN** updater 拒绝该 bundle，不得以 bundle 自带 tag 改写目标 release

#### Scenario: Missing provenance bundle
- **WHEN** 目标 release 缺少 `checksums.txt.sigstore.json` 或 bundle 无法解析
- **THEN** updater fail closed，不得降级为仅校验同源 checksum

### Requirement: Verified staged update
Self-updater SHALL 仅在 Trusted release provenance 验证成功后，于目标 executable 同一文件系统 staging 新 binary，并在替换前完成已验证 release checksum、解包、权限、可执行性和 durable write 验证。

#### Scenario: Candidate passes preflight
- **WHEN** checksum provenance 已验证成功、下载内容与该 checksum 一致且包含适用平台 binary
- **THEN** updater 完成 staging 和 `fsync` 后才进入 replace 阶段

#### Scenario: Candidate fails verification
- **WHEN** provenance、checksum、archive、权限或 executable 验证失败
- **THEN** updater 以非零状态退出，并保持当前 executable 与服务不变

### Requirement: Last-known-good backup
Updater SHALL 在替换前保留当前 executable 的同文件系统 last-known-good backup 及版本 metadata，并通过 atomic rename 安装 staged candidate。Backup MUST NOT 在新版本 health commit 前删除。

#### Scenario: Binary is replaced
- **WHEN** candidate preflight 成功
- **THEN** 当前版本已可从记录的 backup 恢复，且目标路径原子切换到 candidate

#### Scenario: Replace fails
- **WHEN** atomic replacement 无法完成
- **THEN** updater 报告失败，并确保目标路径仍包含 current 或可立即恢复的 last-known-good binary

### Requirement: Durable update transaction
Updater SHALL 持久记录 source/target version、executable、service role/name、backup、当前阶段和最后错误。未提交 transaction SHALL 在下一次 update/recovery 操作前被识别并处理。

#### Scenario: Update is interrupted after replacement
- **WHEN** process 在 health commit 前终止
- **THEN** 后续 updater invocation 检测到 transaction，并要求或执行确定的 recovery/rollback，而不是开始覆盖 backup 的新更新

#### Scenario: Update commits successfully
- **WHEN** target version 通过 health check
- **THEN** updater 标记 transaction committed、清理 staging，并保留一份 previous release 供显式 rollback

### Requirement: Mode-aware local health check
Pure Hub、standalone 与 Agent SHALL 通过本地 Unix HTTP listener 提供包含 role、deployment mode、version、commit、ready 和 process instance 的 health response。Native updater SHALL 在有界超时内同时确认 service manager active、role 匹配、version 等于 target 且 `ready=true`。

#### Scenario: Updated Hub becomes ready
- **WHEN** Hub service restart 后本地 probe 在 timeout 内报告目标版本和 ready
- **THEN** updater 提交更新并报告成功

#### Scenario: Updated pure Hub becomes ready without tmux
- **WHEN** native pure Hub service restart 后报告 `role=hub`、目标版本和 ready，且系统没有 tmux
- **THEN** updater 接受该 role-aware health result，不要求 standalone 的本机 tmux checks

#### Scenario: Updated Agent reports wrong version
- **WHEN** Agent service active，但本地 probe 报告的版本不是 target
- **THEN** updater 将更新视为失败并进入 rollback

#### Scenario: Health probe times out
- **WHEN** restart 后没有在 timeout 内获得匹配且 ready 的 response
- **THEN** updater 进入 rollback，而不是仅凭 restart command 成功宣告更新完成

### Requirement: Automatic rollback after unhealthy restart
Restart failure、service exit、probe timeout、role/version mismatch 或 `ready=false` SHALL 触发自动 rollback。Updater SHALL 恢复 previous binary、再次 restart，并验证 previous version ready。

#### Scenario: New version fails but previous version recovers
- **WHEN** target health check 失败且 previous binary rollback 后通过 health check
- **THEN** updater 以非零状态退出，明确报告 target 失败和 previous version 已恢复

#### Scenario: Rollback also fails
- **WHEN** previous binary 无法恢复或无法变为 ready
- **THEN** updater 保留 backup 和 transaction，返回包含新旧版本状态的可操作错误

### Requirement: Explicit rollback and no-restart semantics
Updater SHALL 提供显式 rollback/recovery 路径。`--no-restart` SHALL 只安装 binary，不声称运行中服务已切换版本，也不执行需要新 process 的 runtime health commit。

#### Scenario: Operator requests manual rollback
- **WHEN** 有 previous release metadata 且 operator 执行 rollback
- **THEN** updater 原子恢复 previous binary，并在需要时 restart 和验证该版本

#### Scenario: Update uses no-restart
- **WHEN** active service 以 `--no-restart` 更新 binary
- **THEN** output 明确区分 installed version 与 running version，并保留后续 restart/health guidance

### Requirement: Update outcome reporting
Updater SHALL 持久记录最近一次 attempt 的 source、target、outcome、时间和非敏感 error summary，供 CLI 与 Fleet Health 读取。

#### Scenario: Update rolls back
- **WHEN** target 失败且 previous version 恢复
- **THEN** recent outcome 记录为 rolled-back，并包含 target 与 restored version

### Requirement: Deployment-aware update strategy
Updater SHALL distinguish native binary installations from the official Docker deployment. In Docker mode, read-only version checking MAY run, but executable replacement, systemd/launchd restart, transaction backup and binary rollback MUST be refused without modifying the image filesystem. Output SHALL provide the supported Compose pull/recreate, health verification and previous image tag/digest rollback procedure.

#### Scenario: Check a Docker deployment
- **WHEN** operator runs the read-only update check in an official Hub container
- **THEN** TmuxAtlas reports current and available versions together with Docker-specific update guidance without staging a binary

#### Scenario: Attempt binary update in Docker
- **WHEN** operator invokes an install/restart update path while `TMUXATLAS_DEPLOYMENT=docker`
- **THEN** updater exits non-zero before download/staging and points to `docker compose pull` plus `docker compose up -d`

#### Scenario: Attempt binary rollback in Docker
- **WHEN** operator invokes the native binary rollback path in official Docker mode
- **THEN** updater refuses to mutate the executable and explains how to recreate from the previous immutable image tag or digest while preserving the volume

#### Scenario: Update a native installation
- **WHEN** systemd or launchd owns a native Hub, standalone or Agent binary
- **THEN** updater continues to use provenance verification, staged replacement, mode-aware health commit and automatic binary rollback

### Requirement: Updater failure-injection verification
项目 SHALL 使用临时 executable、fake service manager 和 fake local health probe 覆盖 replace、restart、timeout、version mismatch、successful rollback、failed rollback 和 interrupted transaction，MUST NOT 操作测试主机的真实服务。

#### Scenario: Updater test suite runs
- **WHEN** CI 执行 updater tests
- **THEN** 每个故障阶段都验证 executable、backup、journal、退出状态和 reported outcome

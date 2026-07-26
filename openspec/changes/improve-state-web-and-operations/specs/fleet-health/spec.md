## ADDED Requirements

### Requirement: Stable Fleet inventory
Fleet Health SHALL 以 stable host ID 维护本机和已知 Agent 的独立 inventory，并展示 display name、role、online observation、last seen 与 last state sync。Display name 重复 MUST NOT 合并 host。

#### Scenario: Fleet contains local Hub and Agents
- **WHEN** Web UI 收到当前 Fleet snapshot
- **THEN** 它按 stable host ID 展示每个已知节点及其 role 和状态时间

#### Scenario: Hosts have duplicate display names
- **WHEN** 两个 host ID 的 display name 相同
- **THEN** Fleet inventory 保留两行独立记录，并允许用户查看各自的稳定身份

### Requirement: Fleet version visibility
Fleet Health SHALL 展示每个 host 的 application version、commit（如可用）和 Hub version，并在版本可比较时产生 `version-behind` 或 `version-ahead` reason。不可解析或缺失的版本 SHALL 标为 unknown，而不是猜测顺序。

#### Scenario: Agent is older than Hub
- **WHEN** Agent 报告的可解析版本低于 Hub version
- **THEN** Fleet Health 显示 `version-behind`、两个版本值和适用的更新指引

#### Scenario: Agent is newer than Hub
- **WHEN** Agent 报告的可解析版本高于 Hub version
- **THEN** Fleet Health 显示 `version-ahead`，且不把该节点错误标为 up to date

#### Scenario: Version cannot be compared
- **WHEN** host 未报告版本或版本格式不可解析
- **THEN** Fleet Health 显示 version unknown，并保留原始值供诊断

### Requirement: Explicit compatibility classification
Fleet Health SHALL 仅依据显式 state/protocol compatibility metadata 判定 `incompatible`，MUST NOT 仅凭 application version 大小推断协议不兼容。

#### Scenario: Reported compatibility range excludes Hub
- **WHEN** host 报告的兼容范围不包含 Hub 当前支持的 schema/protocol version
- **THEN** Fleet Health 显示 `incompatible` 及不匹配的实际值和期望范围

#### Scenario: Compatibility metadata is absent
- **WHEN** host 未报告足够的 compatibility metadata
- **THEN** compatibility 显示 unknown，而不是 incompatible

### Requirement: Freshness and availability health
Fleet Health SHALL 根据既有 online observation、last seen、last state sync 与配置的 freshness threshold 产生 `offline`、`stale` 或 fresh reason，并显示原始时间/年龄。

#### Scenario: Host is explicitly offline
- **WHEN** 当前 canonical state 将 host 标记为 offline
- **THEN** Fleet Health 显示 `offline` 和最后已知时间，不提供会被误解为已执行的恢复状态

#### Scenario: Connected host stops updating state
- **WHEN** host 仍被观察为 online，但 last state sync 超过 freshness threshold
- **THEN** Fleet Health 显示 `stale` 及状态年龄

#### Scenario: Host state remains fresh
- **WHEN** host online 且在 threshold 内更新 state
- **THEN** Fleet Health 将 freshness 标为 fresh

### Requirement: Agent and updater health facts
Fleet Health SHALL 在可用时展示 Agent/hook check、deployment mode、image tag/digest，以及最近 native updater 的 source version、target version、outcome、时间和错误摘要。缺失事实 SHALL 显示 unknown，MUST NOT 被当作成功。

#### Scenario: Agent hooks need setup
- **WHEN** host health facts 报告某个已安装 Agent 的 hook 未配置
- **THEN** Fleet Health 显示对应 check failure 和 host-specific setup guidance

#### Scenario: Update rolled back
- **WHEN** host 最近一次 updater outcome 为 rolled-back
- **THEN** Fleet Health 显示目标版本失败、恢复后的版本和非敏感错误摘要

#### Scenario: No updater result exists
- **WHEN** host 从未记录 updater outcome
- **THEN** Fleet Health 显示 update history unknown，而不是 success

#### Scenario: Hub runs from an official container
- **WHEN** Hub health facts 报告 Docker deployment mode 和 image revision
- **THEN** Fleet Health 展示该 deployment mode 与可用 tag/digest，而不把缺少 native updater transaction 当作失败

### Requirement: Explainable aggregate health
Fleet Health SHALL 保留所有原始 facts 和可组合 reason codes，并显示一个按严重度计算的摘要。`healthy` SHALL 只用于 online、fresh、显式兼容且没有失败 check 的 host。

#### Scenario: Host has multiple health reasons
- **WHEN** 一个 host 同时 version-behind 且 Agent hook check 失败
- **THEN** UI 显示最高严重度摘要，并允许查看两个 reason 及其证据

#### Scenario: Host is fully healthy
- **WHEN** host online、fresh、兼容、版本符合期望且所有已知 checks 通过
- **THEN** Fleet Health 显示 healthy 及最近验证时间

### Requirement: Non-destructive Fleet remediation
Fleet Health SHALL 只提供适合 host role/platform 的诊断、setup 或 updater 命令和复制操作。本 capability MUST NOT 从浏览器执行命令、触发远程更新、修改 pairing 或控制 Peer reconnect。

#### Scenario: User requests remediation guidance
- **WHEN** 用户展开一个 version-behind 或 hook failure
- **THEN** UI 展示可复制的 host-specific 命令及其用途，但不会向该 host 执行命令

#### Scenario: Host is offline
- **WHEN** 用户查看 offline host
- **THEN** UI 只显示最后状态和本地检查建议，不声称已发起 reconnect

#### Scenario: Container Hub needs an update
- **WHEN** version-behind 节点是 official Docker Hub
- **THEN** UI 提供 `docker compose pull`、recreate、health 和固定旧 image rollback 指引，而不是 native binary self-update 命令

### Requirement: Fleet Health uses revisioned state
Fleet Health SHALL 从 canonical revisioned state projection 读取 inventory 和 health facts，并在 reconnect rehydrate 后一次性反映当前 Fleet，而不是混合不同请求时刻的数据。

#### Scenario: Browser reconnects after Fleet changes
- **WHEN** 断线期间 host 版本或健康状态发生变化
- **THEN** reconnect snapshot 使 Fleet Health 在 ready 前更新到同一 revision 的当前结果

### Requirement: Fleet Health verification
项目 SHALL 包含自动化测试，覆盖 duplicate display names、version ordering、explicit incompatibility、stale/offline thresholds、unknown facts、updater outcome 和 non-executing remediation。

#### Scenario: Fleet fixtures are evaluated in CI
- **WHEN** CI 使用多 host fixture 运行 Go 与 browser tests
- **THEN** 每个 health reason 都由其明确事实触发，且 UI 不合并同名 host 或发起远程命令

## ADDED Requirements

### Requirement: Pure Hub construction proof
项目 SHALL 通过可观察的 role-construction tests 证明 `tmuxatlas hub` 在启动与服务期间没有查找 tmux executable、连接 tmux socket、构造 local state producer 或创建本机 PTY handler。

#### Scenario: Start Hub with tmux absent
- **WHEN** 集成测试在 PATH 中没有 tmux 的环境启动 `tmuxatlas hub`
- **THEN** Hub 本地 health 在有界时间内报告 `role=hub`、`ready=true`，且日志不包含 tmux lookup 或 local integration retry

#### Scenario: Address a local target
- **WHEN** 纯 Hub 收到以自身 identity 为 host 的 Session mutation 或 PTY 请求
- **THEN** 请求返回 structured unsupported/not-found outcome，且不会执行本机命令

### Requirement: Runtime role health identity
Hub 与 standalone SHALL 向本地 Unix health endpoint 报告准确的 role 和 deployment；Docker Hub MUST 报告 `role=hub` 与 `deployment=docker`。

#### Scenario: Probe Docker Hub
- **WHEN** 容器 healthcheck 通过私有 Unix socket 请求 `/health`
- **THEN** 响应包含当前 version、commit、instance、`role=hub`、`deployment=docker` 和 `ready=true`

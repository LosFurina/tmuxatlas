## Why

TmuxAtlas 的 Hub 同时承载浏览器 API、浏览器 WebSocket、Peer 配对与控制通道，以及本机 Agent hook 事件。当前部分入口的信任边界没有被明确隔离：原本供本机 CLI 通过 Unix socket 调用的 `/api/tool-event` 同时暴露在公网 TCP Router；公共 Passkey ceremony、Peer pairing 和 WebSocket 入口缺少统一的资源限制；现有 pairing code、Ed25519 输入校验、bootstrap token 生命周期和浏览器状态变更防护也不足以抵御公网暴力、重放、资源耗尽和 same-site cross-origin 请求。

这些入口具有终端控制、Peer 信任建立和通知发送能力，应由应用自身实施 fail-closed 的安全边界，而不能仅依赖 Cloudflare Tunnel、Nginx 或部署者经验。

## What Changes

- **BREAKING** 将本机 hook 事件入口与公网 Router 分离：`/api/tool-event` 默认只在用户私有 Unix socket 上提供，移除 `tmuxatlas notify` 未认证 TCP fallback；本机通知必须使用 Unix socket。
- 新增统一的 Public Ingress 安全层：
  - 为 HTTP 请求设置全局上限和更严格的逐路由 body 上限。
  - 为公共认证、bootstrap、pairing 及 WebSocket upgrade 设置按来源的速率限制、并发限制和短时 burst 容量。
  - 为浏览器、Peer control 和 Peer PTY WebSocket 设置握手期限、最大消息大小、最大连接数及异常连接回收策略。
  - 对超限请求返回稳定且不泄漏内部状态的错误，并记录可聚合的安全审计指标。
- 加固 cookie-authenticated HTTP mutation：
  - 所有浏览器状态变更请求必须来自与 `TMUXATLAS_PUBLIC_URL` 完全一致的 Origin。
  - 要求 Session 绑定的 CSRF token 或等价的不可跨源伪造 header。
  - JSON API 必须接受明确的 `application/json`，拒绝利用 `text/plain` 等 CORS-safelisted content type 发送 JSON。
  - Unix socket 请求及具有独立密码学认证的非浏览器协议使用明确、最小化的豁免路径。
- 提升 pairing 安全性：
  - 使用无 modulo bias 的随机抽样，并将 pairing code 的最小熵提高到足以抵抗完整在线枚举的水平。
  - 保留短期有效和一次性语义，同时限制每个来源及全局的失败次数、并发尝试和 pending code 数量。
  - Pairing completion 必须包含对 domain-separated pairing transcript 的 Ed25519 签名，以证明提交者持有对应私钥。
  - Code 校验、持有证明、一次性消费和 Peer 持久化必须具有原子成功语义；同一 code 的并发或重放请求至多一个成功。
  - 错误响应不得泄漏 code 是否存在、是否过期或哪一步密码学验证失败。
- 严格验证 Ed25519 身份材料：
  - 在持久化及使用前严格验证 Base64 编码、公钥长度、签名长度和公私钥一致性。
  - 畸形 Peer 记录必须产生可操作的启动或诊断错误，不得进入认证路径或触发 panic。
  - 对重复公钥、冲突身份和畸形名称实施确定的拒绝策略。
- 收紧首次 Passkey bootstrap token 生命周期：
  - Token 仅在没有任何 Passkey 时存在，具有明确且较短的有效期，并只以摘要形式保存在内存。
  - Token 只通过本机管理通道或一次性的受控启动输出交付，不进入常规请求日志或长期重复日志。
  - Token 必须绑定到单个 registration ceremony；成功、过期、取消或失败后均不可重放。
  - Token 过期或 ceremony 失败后，可通过本机管理操作安全轮换，无需移动 credential store 或依赖进程重启。
  - 首个 Passkey 成功持久化后，所有 bootstrap token 和未完成 bootstrap ceremony 立即失效。
- **BREAKING** 防止无认证公网误配：
  - `--no-auth` 仅允许与 loopback listener 和 localhost/loopback Public URL 同时使用。
  - 当 Public URL 表示外部 HTTPS origin、监听地址非 loopback，或配置明显处于反向代理部署时，启动必须拒绝 `--no-auth`，而非仅打印警告。
  - TCP 请求的 `Host` 必须符合配置的 Public URL 或明确允许的本机开发 Host，降低 DNS rebinding 和错误虚拟主机转发风险。
- 增加覆盖 TCP/Unix socket 入口隔离、请求与连接上限、pairing 暴力与重放、畸形 Ed25519 key、bootstrap token 生命周期、same-site cross-origin CSRF 以及不安全 `--no-auth` 配置的单元、集成和浏览器 E2E 测试。

## Capabilities

### New Capabilities

- `public-ingress-security`: 定义 TCP 与 Unix socket 的入口隔离、HTTP/WS 大小和资源限制、速率与并发控制、浏览器 Origin/Content-Type/CSRF 策略、安全错误响应，以及本地 hook ingest 的信任边界。

### Modified Capabilities

- `peer-transport`: 提升 pairing code 熵，增加在线暴力保护、一次性原子消费、重放防护、Ed25519 私钥持有证明，以及身份材料的严格编码和长度校验。
- `passkey-management`: 补充首次 Passkey bootstrap token 的生成、交付、过期、轮换、ceremony 绑定和最终失效要求，并为公共 WebAuthn ceremony 增加入口滥用保护。
- `proxy-deployment`: 将公网部署的认证要求、配置 Host 校验和 `--no-auth` fail-closed 约束纳入受信网关边界，明确反向代理不能替代应用层入口保护。

## Impact

- Hub Router、HTTP server、Unix socket server 及中间件，主要涉及 `pkg/server/`、`pkg/auth/` 和 `pkg/socket/`。
- Pairing code、Peer store、Ed25519 验证和 pairing handler，主要涉及 `pkg/identity/` 与 `pkg/peer/`。
- `tmuxatlas notify` 将不再通过未认证 HTTP fallback 投递事件；依赖 `--server` 的现有 hook 配置需要迁移到 Unix socket。
- Browser API 调用需要携带受 Session 绑定的 CSRF 信息，并使用严格的 JSON Content-Type；相关 WebAuthn 和设置请求需要同步适配。
- `--no-auth` 的可用范围收窄到明确的本机开发场景。通过 Cloudflare Tunnel、Nginx 或其他网关公开访问的部署必须启用 TmuxAtlas 身份认证。
- 现有合法 Passkey 和格式正确的 Ed25519 Peer identity 不需要迁移。持久化的畸形或不完整 Peer key 将被拒绝，并通过 `doctor` 或启动错误提示修复或重新配对。
- Pairing code 是短期数据，格式与强度变化不需要磁盘迁移，但旧版本生成的 pending code 在升级或重启后失效。
- 需要新增单元测试、并发测试、反向代理集成测试及浏览器安全 E2E，并更新 CLI、部署和安全文档。
- 本 change 不包含 Peer 运行时连接生命周期、Peer 在线状态同步、浏览器状态管理、前端交互优化或终端功能改造。

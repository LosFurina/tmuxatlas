## 1. 安全基元与持久化校验

- [x] 1.1 在 `identity` 包实现统一的严格 Ed25519 解析与验证，覆盖规范 Base64、32-byte 公钥、64-byte 私钥/签名以及公私钥一致性，并确保所有签名和验签入口先校验长度
- [x] 1.2 为本机 identity 与 Peer store 增加名称、重复公钥、冲突名称和畸形记录校验，并让启动及 `doctor` 返回可定位且不自动改写信任数据的诊断
- [x] 1.3 建立可由测试注入时钟和阈值的 ingress policy 基元，包含 token bucket、全局/分类并发 semaphore、稳定错误映射、无敏感字段的聚合计数器及有限默认值

## 2. 公共 Router 与本机事件边界

- [x] 2.1 将 HTTP 注册拆分为 public Router 和 local Unix-socket Router，仅在 local Router 注册 `/api/tool-event`，并验证 TCP 请求不会记录事件或触发 Web Push
- [x] 2.2 移除 `tmuxatlas notify` 的未认证 HTTP/HTTPS fallback 及相关参数路径，更新生成的 hook 配置，使 Hub 和 Agent 通知都只使用 Unix socket
- [x] 2.3 在创建和复用 Unix socket 时校验当前用户所有权及私有目录/socket 权限，并补充不可连接、权限错误和正常本机投递测试

## 3. HTTP 资源边界与浏览器 mutation

- [x] 3.1 为公共 `http.Server` 配置有限 header 大小、header/body 读取期限和全局 body 上限，并实现只接受单个 JSON 值的有界 decoder
- [x] 3.2 为 WebAuthn、bootstrap、pairing、设置及其他 JSON mutation 分配可测试的逐路由 body 上限和 `application/json` 校验，统一验证 413、415 及无部分 mutation 行为
- [x] 3.3 为每个认证 Session 生成并常量时间校验 CSRF token，实现与规范化 `TMUXATLAS_PUBLIC_URL` 完全一致的 Origin middleware，并建立 Unix socket、Peer 与 pre-Session WebAuthn 的最小显式豁免表
- [x] 3.4 将 Web 前端 mutation 收敛到共享请求封装，自动附带 CSRF header 和 JSON Content-Type，并覆盖 same-site cross-origin、缺失/错误 Origin、`text/plain` 及有效 same-origin 请求

## 4. WebSocket 资源生命周期

- [x] 4.1 为 browser terminal、Peer control 和 Peer PTY 分别实施 upgrade 限速、握手期限、全局/分类连接配额，并保证认证或 upgrade 失败释放配额
- [x] 4.2 为三类 WebSocket 设置协议适配的有限 read limit、ping/pong 或活动 deadline 与异常关闭清理，确保超大消息和僵尸连接不保留资源
- [x] 4.3 增加 WebSocket 并发、超限消息、超时、异常断开和配额恢复的 Go 集成测试，验证一个类别耗尽不会阻塞其他类别

## 5. Pairing 协议加固

- [x] 5.1 将 pairing code 改为基于 256 词表直接均匀抽样的六词格式，保留 5 分钟 TTL，限制 pending 数量，并用确定性随机源测试 48-bit 最小熵和无 modulo bias 路径
- [x] 5.2 在 pairing client/server 实现版本化、domain-separated、长度前缀的 canonical transcript，并要求提交者以对应 Ed25519 私钥签名后才能完成 pairing
- [x] 5.3 将 code 状态迁移、proof 校验、身份冲突检查、Peer 原子持久化及一次性消费组合为原子成功流程，保证失败可恢复 pending、并发至多一个成功且重放失败
- [x] 5.4 对 pairing generation/completion 实施每来源与全局速率、burst 和并发限制，统一 code 不存在、过期、签名错误及冲突身份的远程失败形状，并补充暴力与敏感日志测试

## 6. Passkey ceremony 与 bootstrap 生命周期

- [x] 6.1 将 WebAuthn ceremony store 改为容量有界、每请求清理工作有界的结构，并在创建 ceremony 前执行 begin/finish 的每来源、全局速率和并发检查
- [x] 6.2 实现仅在无 Passkey 时存在的 256-bit bootstrap token：内存只存摘要、10 分钟过期、原子绑定单个 registration ceremony，并在成功、失败、取消或超时后阻止重放
- [x] 6.3 在首个 credential 持久化后清空全部 bootstrap 状态，并通过同用户 Unix socket 提供无需重启的 token 轮换命令；验证公网无轮换路由且已有 Passkey 时拒绝轮换

## 7. Host、无认证模式与升级文档

- [x] 7.1 将 listener、Public URL 和认证模式做联合启动校验，仅允许 loopback listener 配合 localhost/loopback origin 使用 `--no-auth`，其余组合以可操作错误 fail closed
- [x] 7.2 在 public Router 校验规范化 HTTP `Host`，并将浏览器 WebSocket Origin/Host 同时绑定到 Public URL；默认忽略 `X-Forwarded-Host`、`X-Forwarded-For` 等不受信安全信号
- [x] 7.3 更新 Cloudflare Tunnel、Nginx、CLI、hook 与升级文档，说明 Host 保留、应用层认证/限额、Unix-only notify、新 pairing code、bootstrap 轮换和不安全 `--no-auth` 配置迁移
- [x] 7.4 增加旧 hook/CLI 配置及错误代理 Host 的迁移诊断，确认升级不改写合法 Passkey/Peer identity，旧 pending pairing code 只需重新生成，并记录可执行的回滚步骤

## 8. 综合验证与发布门禁

- [x] 8.1 为新增安全基元、Router、CSRF、identity、pairing、bootstrap 和配置校验补齐表驱动 Go 单元测试，并运行 `go test ./...`
- [x] 8.2 对 ceremony/pairing 并发消费、Peer store mutation、限流器和 WebSocket 配额运行有针对性的 `go test -race`，修复全部数据竞争
- [x] 8.3 扩展反向代理集成套件，覆盖 TCP/Unix tool-event 隔离、Host/Origin、HTTP 大小/速率、Peer pairing proof、control/PTY WebSocket 及 Cloudflare/Nginx 等价转发行为
- [x] 8.4 扩展 Chromium virtual-authenticator E2E，覆盖 bootstrap 过期/轮换、正常注册登录、CSRF token、same-site cross-origin 和错误 Content-Type，使用隔离配置目录
- [x] 8.5 运行 `cd web && npm run build`（含 TypeScript typecheck）、完整 Go/浏览器测试及 `openspec validate harden-public-ingress-and-pairing --strict`，记录有限默认阈值并确认发布文档与实现一致

## Context

TmuxAtlas Hub 当前在同一个 HTTP Router 上承载浏览器 API、浏览器 WebSocket、Peer pairing/control/PTY，以及原本只供同一用户本机 hook 使用的 tool-event ingest。浏览器侧依赖 Session cookie 和 WebAuthn，Peer 侧依赖 Ed25519，Agent hook 则只应依赖用户私有 Unix socket；这些入口的信任模型不同，却没有在路由层完全隔离。

现有公共 ceremony、pairing 和 WebSocket 也缺少统一的请求大小、速率、并发及连接生命周期限制。三词 pairing code 只有约 18 bit 搜索空间，pairing completion 不证明提交者持有公钥对应的私钥；Ed25519 输入在进入持久化和验证路径前也没有统一验证。首次 Passkey bootstrap token 则会一直有效到注册成功。与此同时，`SameSite=Strict` 不能阻止同站不同源请求，`--no-auth` 还能与外部 Public URL 或非 loopback listener 组合。

实现必须继续支持 Cloudflare Tunnel、Nginx、普通系统信任 HTTPS/WSS、现有合法 Passkey 和 Peer identity，并避免把安全性寄托在代理默认限制或不受信的转发头上。

## Goals / Non-Goals

**Goals:**

- 在 Router 层分离公共 TCP 和用户私有 Unix socket，只允许后者接收本机 hook 事件。
- 为公共 HTTP、WebAuthn、pairing 和 WebSocket 建立应用内、可测试的资源边界。
- 为 cookie-authenticated mutation 同时实施精确 Origin、Session 绑定 CSRF token 和 JSON Content-Type 校验。
- 将 pairing 提升为高熵、限速、一次性且证明 Ed25519 私钥持有的协议。
- 在身份材料进入存储或密码学 API 前验证编码、长度、一致性和冲突。
- 让首次 Passkey bootstrap secret 短期、单 ceremony、可在本机安全轮换。
- 对不安全的 `--no-auth`、Host 和代理配置 fail closed。

**Non-Goals:**

- 代替 Cloudflare、Nginx 或操作系统防火墙提供 DDoS 清洗。
- 改变 WebAuthn RP/user 模型、Passkey 管理 UX 或 credential 文件格式。
- 引入账户、角色、多用户恢复流程或远程 bootstrap 管理。
- 改造 Peer 在线状态、断线重连、移除 Peer 后的运行时撤销或 PTY capability 协议。
- 在本 change 中实现浏览器 Session 注销后主动关闭已升级 WebSocket。

## Decisions

### 1. 用两个显式 Router 固化本机与公网信任边界

公共 TCP server 使用 public Router；Unix socket server 使用 local Router。两者可以复用无副作用的 handler，但 `/api/tool-event` 只注册到 local Router。`tmuxatlas notify` 只连接用户私有 Unix socket，不再接受或尝试未认证 TCP fallback。socket 目录和 socket 文件在创建时校验为当前用户私有。

选择路由隔离而不是在 handler 内判断 `RemoteAddr`，因为代理、测试 transport 和 Unix connection 的地址表达不同，条件判断容易在后续重构时被绕过。

### 2. 在协议解析前实施分层资源预算

HTTP server 设置有限的 header 大小和 header/body 读取期限；公共 Router 先应用全局 body 上限，再由 pairing、WebAuthn 等路由应用更小的上限，并使用 `http.MaxBytesReader` 后只解码一个 JSON 值。公共 authentication、bootstrap、pairing 与 WebSocket upgrade 同时受每来源和全局 token bucket、并发 semaphore 及有限 burst 约束。来源默认只取直接 socket peer IP，不信任 `X-Forwarded-For` 或同类头；反向代理导致来源合并时，全局预算仍提供保护。

WebSocket 在 upgrade 前占用连接配额，设置握手期限；upgrade 后按协议设置有限 read limit、ping/pong deadline 和异常连接回收。浏览器 terminal、Peer control 与 Peer PTY 使用独立配额，避免一个入口耗尽全部连接。限制使用有界默认值并集中定义，使测试可缩小阈值而不依赖大量流量。

替代方案是只记录告警或依赖反向代理限制；这无法保护直连、错误代理配置或应用内 ceremony map，因此不采用。

### 3. 用 Session 绑定 token 与精确 Origin 保护浏览器 mutation

认证成功时为 Session 生成独立的随机 CSRF token，并通过同源 session/status 响应交给前端；前端 mutation wrapper 使用 `X-TmuxAtlas-CSRF` 回传。所有 cookie-authenticated `POST`、`PATCH`、`PUT` 和 `DELETE` 必须同时满足：

- `Origin` 的 scheme、host 和有效 port 与规范化后的 `TMUXATLAS_PUBLIC_URL` 完全一致；
- CSRF header 与当前 Session 常量时间比较一致；
- 有 JSON body 的路由使用明确的 `application/json` media type。

WebAuthn login 和首次 bootstrap 在建立 Session 前没有 CSRF token，因此使用精确 Origin、ceremony cookie、WebAuthn origin/RP 验证和入口限速的组合。Pair 协议、Peer WebSocket 和 Unix socket 具有独立信任机制，走显式豁免路径而不是通用浏览器 mutation middleware。Fetch Metadata 仅用于拒绝明显的 cross-site 流量，不作为授权依据。

只依赖 `SameSite=Strict` 或自定义 header 均不足：前者允许 same-site cross-origin，后者若没有 Origin/Content-Type 规则容易产生遗漏。

### 4. Pairing 使用 48 bit 均匀词组和签名 transcript

新 code 使用密码学随机源，从固定的 256 词表中直接以随机 byte 均匀选择六个词，获得 48 bit 最小熵；TTL 保持 5 分钟。PairingManager 对 pending code 数量设硬上限，并在生成、验证和消费时清理过期项。

completion 请求携带 code、规范化名称、规范编码的 Ed25519 公钥和签名。签名覆盖带版本和 domain separator 的无歧义 transcript，包括规范化 Hub origin、规范化 code、peer 名称及原始公钥：

`TMUXATLAS-PAIR-V1 || length(origin) || origin || length(code) || code || length(name) || name || publicKey`

Hub 先完成语法和长度检查，再在 PairingManager 的单次消费事务中校验 code、验证签名、检查身份冲突并持久化 Peer。并发请求中的 code 状态从 pending 原子转为 processing；只有持久化成功才转为 consumed，失败则在未过期时恢复 pending。成功响应后重放一定失败。

直接扩大词数但继续 modulo 抽样仍会保留偏差；仅依赖限速则把安全性过度交给部署拓扑；仅提交公钥不能证明控制私钥。因此三者都不单独采用。

### 5. 集中验证 Ed25519 identity

identity 包提供唯一的严格解析入口：要求规范 padded Base64、32-byte 公钥、64-byte 私钥、64-byte 签名，并验证本机私钥推导出的公钥等于持久化公钥。Pairing、Peer store load、control challenge 和本机 identity load 都在调用 `ed25519.Sign`/`Verify` 前使用该入口。

Peer 名称 trim 后必须非空、长度有界且不含控制字符。新 pairing 若名称或公钥已存在，一律返回冲突且不改写；加载到重复名称、重复公钥或畸形 key 时启动失败，并由 doctor 报告记录位置和修复建议。外部 pairing 响应保持通用，日志仅记录安全的 fingerprint，不记录 code、签名或私钥。

选择拒绝而不是自动修复持久化身份，因为猜测截断、重新编码或覆盖冲突记录可能改变信任关系。

### 6. Bootstrap token 是有过期时间的本机可轮换 secret

仅当 credential store 为空时生成 256-bit bootstrap token，内存只保存摘要、创建时间和 10 分钟 expiry。启动时明文最多输出一次；常规日志、HTTP 错误和指标不得包含 token。过期后只能通过用户私有 Unix socket 上的本机管理命令轮换并取得新 token，远程 TCP 不提供轮换入口。

registration begin 在校验 token 后立即把它原子绑定到一个 registration ceremony；其他 begin 不能复用。该 ceremony 成功、失败、取消或超时均使 token 失效。首个 credential 成功持久化后，所有 bootstrap token 与 bootstrap ceremony 立即清空；只有 credential store 再次为空时才允许本机生成。

无限期 token 便于无人值守部署，但把短暂初始化凭据变成长期管理 secret，因此不采用。将 token 写入磁盘也会扩大泄露和清理面。

### 7. `--no-auth` 和 Host 校验在启动及请求阶段 fail closed

配置验证将 listener、Public URL 和 auth mode 作为一个整体处理。`--no-auth` 仅允许所有解析后的 listener 地址均为 loopback，且 Public URL host 为 `localhost`、loopback IP 或等价本机名称；外部 HTTPS origin、通配/非 loopback listener 或明显代理部署直接拒绝启动。

Public Router 在路由前校验 HTTP `Host` 等于 Public URL 的规范 authority；仅本机开发配置可接受明确列出的 localhost/loopback 等价 host。浏览器 WebSocket 进一步要求 `Origin` 与 Public URL 完全一致。应用不使用 `X-Forwarded-Host` 决定信任。

仅打印警告无法防止服务文件或代理配置漂移，故不作为安全边界。

### 8. 安全失败保持稳定、可聚合且不泄漏 secret

超限分别使用稳定的 413、415、429 或 503；Origin/CSRF/Host 失败使用通用 403；pairing 的 code 不存在、过期、签名错误和身份冲突对远程调用者使用同一失败形状。日志和计数器按路由类别、拒绝原因及截断后的来源聚合，不记录请求 body、bootstrap token、pairing code、签名或 Push key。

## Risks / Trade-offs

- **[Risk] 反向代理后的请求共享一个直接来源地址，合法流量可能共同触发 per-source 限制** → 为单管理员工作负载保留合理 burst，并同时区分全局与入口配额；不以不受信转发头换取精度。
- **[Risk] HTTP/WS 上限过低会拒绝大型 WebAuthn attestation 或 Peer state** → 按协议设置独立上限，并用实际 browser/多 session fixtures 验证默认值。
- **[Risk] CSRF token 改造遗漏某个前端 mutation** → 集中使用一个 fetch wrapper，并以路由表测试逐个证明保护或显式豁免。
- **[Risk] 新 pairing 格式使升级前生成的 code 无法使用** → code 本就只在内存中短期存在，升级后提示重新生成。
- **[Risk] 严格 key/store 校验使历史畸形记录阻止启动** → 在升级文档与 doctor 中指出具体记录，保留原文件，不自动覆盖信任材料。
- **[Trade-off] Pairing code 从三词增加到六词** → 提供可复制的完整命令/文本，并以显著提高的离线搜索空间换取少量输入成本。
- **[Trade-off] Bootstrap token 过期后需本机操作** → 提供 Unix socket 管理命令，避免重启或移动 credential store。
- **[Risk] 更严格 Host 校验与会改写 Host 的旧代理配置不兼容** → 在启动日志和 403 审计中给出不含 secret 的诊断，并更新 Cloudflare/Nginx 示例。

## Migration Plan

1. 先引入共享 ingress middleware、严格 identity parser 和新测试，但保持现有入口可观察。
2. 切分 public/local Router，升级 `tmuxatlas notify` 与安装脚本，使 hook 只走 Unix socket。
3. 同步发布新的 pairing client/server transcript；升级会清空旧进程中的 pending code，现有合法 Peer 记录保持不变。
4. 启用 CSRF token，迁移前端所有 mutation 到共享请求 wrapper。
5. 启用 bootstrap expiry/local rotation、Host 校验和 `--no-auth` 启动约束。
6. 在反向代理集成测试中验证浏览器、Passkey、Peer control/PTY、限速与错误路径，再发布 breaking release。

回滚到旧版本不会转换 Passkey 或合法 Peer store；需要恢复旧版 CLI/hook 配置并重新生成短期 pairing code。已经由新版本拒绝的畸形 identity 不会被自动修改，因此仍可由操作者显式修复或从备份恢复。

## Open Questions

None. 具体默认 byte、速率、并发和 deadline 常量在实现时根据现有 E2E fixture 固化并写入运维文档，但它们必须始终为有限值且可在测试构造器中覆盖。

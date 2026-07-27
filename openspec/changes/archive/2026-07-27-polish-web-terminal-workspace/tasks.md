## 1. 基线与变更边界

- [x] 1.1 核对 `improve-state-web-and-operations` 已实现的 revisioned state、mobile toolbar、safe-area 与字体能力，记录本 change 复用点并避免重复实现未归档的 `mobile-terminal` capability
- [x] 1.2 运行现有 Web unit、Chromium E2E、mobile WebKit E2E 与 bundle 检查，记录 entry gzip、xterm gzip、total Brotli 和关键页面基线
- [x] 1.3 盘点 App、Terminal、QuickSwitcher、Help、TopBar、StatusBar、Sidebar 与 Settings 中的快捷键、连接状态、Preferences 来源和 DOM listener，形成迁移清单

## 2. 语义 Tokens 与共享 UI 原语

- [x] 2.1 扩展主题系统，增加 surface、elevation、focus、motion、control-height、terminal-chrome 和语义状态 tokens，并移除 Terminal/TopBar 中的固定品牌颜色
- [x] 2.2 调整字体与视觉层级，使界面正文和控制使用可读的 bundled/system UI font stack，Terminal 与必要技术值保留 monospace，并保持现有 Light、Dark、Retro 等主题兼容
- [x] 2.3 实现 Button、IconButton、Tooltip、StatusPill、Kbd 与统一 focus-visible 行为，保证 icon-only control 有 accessible name 和合适触控目标
- [x] 2.4 实现 Dialog/Sheet、Menu、Toast、Skeleton 与 EmptyState，覆盖 focus trap、background inert、Esc close、focus restore 和 `prefers-reduced-motion`
- [x] 2.5 为共享原语补充组件测试，验证键盘操作、ARIA semantics、主题 tokens、reduced motion 和 44×44 CSS px 移动触控目标

## 3. Command Registry 与快捷键隔离

- [x] 3.1 建立唯一 command registry，定义稳定 command ID、label、category、scope、shortcut、enablement 与 handler contract
- [x] 3.2 将 App、Terminal 和全局导航快捷键迁移到 registry dispatcher，并按 overlay、workspace、terminal scope 明确事件优先级
- [x] 3.3 修复 `Ctrl+H`、`Ctrl+L`、`Ctrl+J` 等冲突，确保未登记的 Terminal 控制键原样进入 PTY，已执行的应用命令向 PTY 发送零个额外 byte
- [x] 3.4 让 Help 从 command registry 自动生成命令与快捷键说明，删除重复静态清单
- [x] 3.5 为 registry、scope、enablement、focus restore 和 PTY byte isolation 增加单元测试

## 4. 状态驱动的 Workspace 导航

- [x] 4.1 建立以 stable `host_id + session` 为 key 的 Workspace view model，从既有 projection 派生 running、waiting、done、error、offline、last activity 和 Host 汇总状态
- [x] 4.2 重构 Sidebar 的 Host → Session 层级，加入 Needs Attention、recent、Pin、文本搜索和状态筛选，并避免重名 display name 合并 Host
- [x] 4.3 使用带 namespace 的本地 Workspace 偏好保存 Pin/recent，确保数据仅包含 canonical target 且不会存储 Terminal 输入或 Mobile Composer 草稿
- [x] 4.4 明确关闭浏览器视图只会 detach；当前 Runtime 无终止 capability 时不显示 kill，未来 capability 可用时才显示完整 Host + Session target、危险样式和二次确认
- [x] 4.5 增加同名跨 Host Session、状态汇总、筛选、Pin/recent、detach 与 capability-gated kill 确认的组件测试

## 5. 统一 Command Palette

- [x] 5.1 将 Quick Switcher 改为由 command registry 和 Workspace view model 驱动的 Command Palette，支持 Host、Session、Window、Agent、状态和应用命令搜索
- [x] 5.2 为 Palette 实现分组结果、fuzzy matching、键盘导航、enablement、执行后关闭和触发位置 focus restore
- [x] 5.3 接入 Overview、Settings、New Session、Reconnect、Fullscreen、Zen Mode 与 stable target 导航命令
- [x] 5.4 在窄屏将 Palette 呈现为不超出 Visual Viewport 的 Bottom Sheet，并保证 desktop dialog/listbox 与 mobile sheet 的 ARIA 行为一致
- [x] 5.5 增加跨 Host 同名结果、disabled command、取消操作、键盘执行、移动布局和无 PTY byte 泄漏测试

## 6. Terminal 生命周期、Cockpit 与统一连接状态

- [x] 6.1 拆分 `useTerminal` 的 xterm、socket generation、Clipboard 与 DOM event lifecycle，清理 reconnect 累积 listener、stale `termConnected` 和生产环境 `window.__term`
- [x] 6.2 建立 Hub connection、Agent availability 与 PTY connection 的统一前端状态模型，区分 connecting、rehydrating、connected、reconnecting、hub-offline、agent-offline、session-ended 和 auth-required
- [x] 6.3 实现紧凑 Terminal Cockpit，显示 Host / Session / Window / Pane、Agent/Tool 状态、连接状态和 Search、Copy、Paste、字号、scroll-to-bottom、Fullscreen/Zen、更多菜单
- [x] 6.4 清理 TopBar、StatusBar 与 Connection Notice 的重复或矛盾状态，把常规遥测移回 Overview/Diagnostics，并保留异常恢复动作
- [x] 6.5 实现真正 Zen Mode，隐藏非必要 Sidebar、TopBar、StatusBar 与 Alert chrome，保持同一 PTY generation、safe-area 和可发现退出路径
- [x] 6.6 实现 scroll-follow latch：用户位于底部时自动跟随，查看历史输出时保持位置并显示新输出/回到底部入口
- [x] 6.7 为 100 次 reconnect listener/timer 上限、状态映射、Cockpit target 更新、Zen generation 保持和 scroll latch 增加测试

## 7. Terminal Search、Clipboard 与右键菜单

- [x] 7.1 接入 xterm Search addon 或等价实现，并配置为首次打开 Search 时才加载的独立 chunk
- [x] 7.2 实现 Search UI 的查询、匹配数、大小写、上一个/下一个、加载失败重试、关闭恢复 Terminal focus 和 target 切换清理
- [x] 7.3 实现 Terminal Context Menu，按 selection、Clipboard capability 和 OPEN target 提供 Copy、Paste、Find 与 Select All
- [x] 7.4 修正 Clipboard 的 generation guard 与反馈：Copy 只复制 selection，Paste 仅由 user gesture 读取，权限拒绝、空值、API 不可用或 target 改变时发送零内容
- [x] 7.5 为多行 Paste 增加明确确认，并根据当前 Terminal bracketed-paste mode 使用兼容输入序列
- [x] 7.6 增加 Search lazy-load/lifecycle、Context Menu、Copy/Paste success/error、multi-line confirmation 和 stale generation 测试

## 8. Mobile Input Composer

- [x] 8.1 在 Terminal input hook 中实现独立 `sendCommand`/`sendRawInput`，捕获 stable target 与 socket generation，绕过 Ctrl/Alt modifier encoder，并只向仍匹配的 OPEN socket 发送
- [x] 8.2 将 `TextEncoder(value + "\r")` 作为单个 Binary WebSocket frame 发送，允许空正文只发送 CR，并按 UTF-8 byte length 拒绝超过 65,535-byte 的正文
- [x] 8.3 实现仅存页面内存、按 stable `host_id + session` 隔离的草稿 Map；切换 target 可恢复各自草稿，logout、unmount 和 reload 不持久化，断线/重连不自动发送
- [x] 8.4 实现 mobile/coarse-pointer 下可收起、自动增高 1–3 行的 Composer `textarea` 与至少 44×44 CSS px 的“发送”按钮，普通 Enter 保留换行，可选 `Cmd/Ctrl+Enter` 提交
- [x] 8.5 关闭 Composer 的 autocorrect、autocapitalize 和 spellcheck，正确处理中文/Unicode/emoji 与 IME composition，composition 未结束时不得提交
- [x] 8.6 成功发送后只清空对应 target 草稿并清除 one-shot modifier；locked modifier 保持可见且不改变 payload，失败或超限时保留草稿并显示 `role=alert` 可恢复反馈
- [x] 8.7 为 raw frame 增加 byte-level 单元测试，精确覆盖前后空格、引号、Shell 元字符、多行、中文/emoji、空输入、单个末尾 CR、modifier bypass 和 `send()` 异常
- [x] 8.8 增加 65,535/65,536 UTF-8 bytes 边界、closed/stale generation、同名跨 Host target、成功清空、失败保留、IME 和 Composer focus 的组件测试

## 9. 响应式 Shell、Preferences 与页面状态

- [x] 9.1 将全局 `touch-action: none` 限定到需要接管手势的 xterm surface，为 Overview、Settings、Sidebar、Sheet 和其他滚动区恢复原生 pan/zoom
- [x] 9.2 使用 Visual Viewport height/offset、safe-area 和合并调度完善 mobile shell、Drawer、Toolbar、Composer 与 Terminal fit，覆盖 portrait、landscape 和软键盘
- [x] 9.3 为 Sidebar Drawer 和其他 overlay 补齐 backdrop、focus trap、background inert、Esc close 与 focus restore，并消除 320px 起的意外横向溢出
- [x] 9.4 统一 App、Sidebar 和 Settings 的 Preferences 来源与优先级，使 Default View、Sidebar Default 等已保存设置真正生效
- [x] 9.5 为 Preferences 乐观更新增加串行化、失败回滚或未保存状态、Retry 与 Toast，禁止吞掉非 2xx 或网络错误
- [x] 9.6 区分 Auth/State/Terminal 的 Loading、Ready-but-empty、stale 和 Error；为真实空 Workspace 提供 New Session 与 Setup CTA
- [x] 9.7 将 Settings、Setup、Overview 和其他非首屏大视图按需拆包，并确认未进入 Session 时不加载 xterm、未搜索时不加载 Search addon

## 10. E2E、视觉回归与发布门禁

- [x] 10.1 扩展 Playwright PTY fixture，使测试可捕获并断言浏览器发出的 Binary input frame、target 和 connection generation
- [x] 10.2 增加 Chromium Workspace E2E，覆盖 Sidebar 状态导航、Command Palette、Cockpit、Search、Context Menu、连接恢复、Alert/Toast 与 Zen Mode
- [x] 10.3 增加 mobile WebKit Composer E2E，精确断言 Unicode、前后空格、多行和空正文均只产生一个 `UTF8(value + "\r")` frame
- [x] 10.4 增加 local/remote 同名 Session 与 generation race E2E，证明草稿和输入不会跨 Host、Session 或 reconnect generation 泄漏
- [x] 10.5 增加 320×568、390×844 与 844×390 的 safe-area、orientation、软键盘、Drawer、Palette、Toolbar 和 Composer 回归
- [x] 10.6 对主要 desktop/mobile views 运行 Axe，并将 critical 与 serious violations 都设为失败门禁
- [x] 10.7 为 1440×900、390×844 和 844×390 的代表性 Light、Dark 与 Retro Workspace 建立可审核的 visual regression 基线
- [x] 10.8 更新用户文档和 Help，说明 Command Palette、Terminal Search、Zen Mode、Clipboard、多行 Paste 与 Mobile Input Composer 的原样发送行为和目标安全边界
- [x] 10.9 运行完整 Web unit/component/E2E、Go 回归、`openspec validate --strict`、`git diff --check` 与 bundle budget；确认无后端协议变化且所有门禁通过

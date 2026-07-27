## Context

TmuxAtlas 的前端已经具备 revisioned state、按 Host 区分的 Session、xterm 懒加载、移动端特殊键工具栏、主题与 PWA，但导航、Terminal、TopBar、StatusBar、通知和设置仍分别实现状态与交互。当前只渲染一个活动 Terminal；切换 target 会更新 Terminal/PTY 生命周期。`useTerminal` 同时承担 xterm、WebSocket、移动 modifier、Clipboard 和 DOM event，应用快捷键则分散在 App、Terminal、QuickSwitcher 与 Help 中。

本 change 只重构浏览器侧 Terminal Workspace，不改变 Hub/Agent protocol、PTY frame、Passkey 或 PWA network-only 语义。实现需要建立在 `improve-state-web-and-operations` 已完成的 mobile toolbar 和 revisioned state 基线上；该 change 尚未归档，因此这里以新 capability 描述增量，避免并行修改 `mobile-terminal` delta。

现有前端 bundle budget 已接近上限，尤其 xterm chunk 约使用 94%。设计必须使用轻量原生语义组件和按需加载，不引入完整 UI framework、图标库或默认 WebGL 负担。

## Goals / Non-Goals

**Goals:**

- 让 Terminal 成为视觉中心，并以 Host、Session、Agent 状态驱动导航与注意力层级。
- 用统一 Command Palette、command registry、Terminal Cockpit 和状态模型减少重复 chrome。
- 补齐 Search、Clipboard、Context Menu、scroll latch、Zen Mode 和可靠的异步反馈。
- 在手机端提供可编辑完整文本、显式发送且严格 target/generation-safe 的 Mobile Input Composer。
- 建立可复用的 UI primitives、响应式、无障碍、视觉回归和 bundle gate。
- 保持 tmux/TUI 原始字节行为及 local/remote Session 输入隔离。

**Non-Goals:**

- 不实现 Warp Blocks、Shell parser、命令历史同步或 Shell injection。
- 不在浏览器中重建 tmux pane、自由分屏、文件管理器、编辑器或 AI Chat。
- 不增加多人协作、键盘广播、Session replay、离线 Terminal 或服务端草稿。
- 不修改 Hub/Agent/Peer protocol、PTY framing、认证或数据库。
- 不在本 change 全面重做品牌、主题市场或引入大型组件体系。

## Decisions

### 1. 使用轻量语义 primitives，而不是 UI framework

在 `web/src/components/ui` 或等价目录建立 Button、IconButton、Tooltip、Dialog/Sheet、Menu、StatusPill、Toast、Skeleton、EmptyState 和 Kbd。组件使用原生 button/dialog semantics、现有 CSS/Tailwind 与主题变量，新增 `surface`、`elevation`、`focus`、`motion`、`control-height`、`terminal-chrome` 和语义状态 tokens。

界面 chrome 使用可读的现有 bundled/system sans stack，Terminal 与少量技术指标保留 monospace；Retro CRT 继续作为可选品牌主题。所有颜色、阴影和 focus ring 必须来自 tokens，状态不得只靠颜色表达。

选择该方案是为了统一交互和无障碍，同时守住 bundle budget。引入 Radix、MUI、完整 icon pack 或 Storybook 作为首要依赖会显著增加入口体积，暂不采用。

### 2. 一个 command registry 统一 Palette、快捷键和 Help

每个应用命令声明稳定 ID、label、category、可选 shortcut、scope、enablement 和执行函数。Command Palette 与 Help 从 registry 派生，只有 registry 的 shortcut dispatcher 能拦截应用级按键。

Terminal 获得焦点时，普通输入和常见 `Ctrl+<key>` 默认直通 PTY；应用快捷键必须显式登记为不冲突组合，并在执行时阻止对应事件继续写入 PTY。Dialog/Palette 打开时由最上层交互 scope 接管键盘。该模型替代 App、Terminal 与 Help 各自维护快捷键的做法，特别修复 `Ctrl+H`、`Ctrl+L` 和 `Ctrl+J` 的冲突或双重执行风险。

### 3. Workspace 导航只消费既有 canonical state

Sidebar 使用稳定 `host_id + session` 作为列表 key 和导航 target，不按 display name 合并 Host。状态 view model 从现有 Hub projection、Agent availability、tool events、last activity 与 PTY state 派生，不增加新状态协议。

导航先显示需要处理的 Session，再显示 recent/pinned 与其余 Host 分组；支持按 Host、Session、Agent 类型和状态查询。Pin/recent 属于浏览器 Workspace 偏好，使用带 namespace 的本地存储并以 canonical target 为 key；不上传命令内容。无法从现有 state 确定 `waiting/done/error` 时，UI 显示更保守的 online/activity 状态而不猜测。

Detach 只关闭浏览器 target。当前 Runtime 没有既有 kill-session API，因此本 change 不新增后端操作，也不显示虚假的 Kill Session 菜单；UI 明确说明关闭视图不会停止 tmux。未来 Runtime 明确声明终止 capability 后，Kill Session 才能使用独立危险动作、展示准确 target 并二次确认，绝不复用关闭 tab/导航按钮。

### 4. Terminal Cockpit 与统一状态模型

Terminal 上方使用单行、可响应折叠的 Cockpit 展示 Host / Session / Window / Pane、当前 Agent/Tool 状态、连接状态及 Search、Copy、Paste、字号、Fullscreen/Zen 和更多操作。TopBar 与 StatusBar 不再重复展示同一连接状态；CPU/MEM 等遥测留在 Overview/Diagnostics。

浏览器把 Hub state、target availability 与 PTY connection 映射为可判定的状态联合：

`connecting` → `connected` → `reconnecting` / `hub-offline` / `agent-offline` / `session-ended` / `auth-required`。

异常状态展示原因和适用的 Retry、Reload 或 Sign in 动作；重连完成、Clipboard、创建/重命名和 Preferences 保存通过统一 Toast/inline feedback 表达。Loading、Ready-but-empty 与 Error 必须分开，避免 snapshot 尚未就绪时显示“No sessions”。

### 5. Search 和可选 Terminal 工具按需加载

第一次打开 Search 时动态加载 xterm Search addon，创建独立 chunk，并把 addon lifecycle 绑定到当前 xterm generation。Search 支持 query、匹配数、上一个/下一个与关闭后恢复 Terminal focus；target 切换或 Terminal dispose 时清理查询与 listener。

WebGL、Serialize、Ligatures 等不属于本 change 的默认依赖；只有独立 benchmark、fallback 和 budget 通过后才能另行加入。链接继续使用现有 WebLinks 机制，并通过显式用户动作安全打开。

### 6. Terminal 输入分为键盘、Paste 与 Mobile Composer 三条明确路径

物理键盘和特殊键工具栏继续走会应用 Ctrl/Alt modifier 的 `sendInput`。Clipboard Paste 只在用户 gesture 下读取，Promise 完成时再次验证 target/generation；多行输入显示确认，并使用 xterm/bracketed-paste 兼容路径。

Mobile Input Composer 新增专用 `sendCommand`/`sendRawInput` 路径，绝不经过 modifier encoder。UI 使用 mobile/coarse-pointer 场景下可收起、自动增高的 1–3 行 `textarea` 和至少 44×44 CSS px 的“发送”按钮；普通 Enter 插入换行，按钮与可选 `Cmd/Ctrl+Enter` 提交，IME composition 尚未完成时快捷键不得提交。自动纠错、自动大写和拼写替换默认关闭。

提交时执行以下原子逻辑：

1. 捕获 canonical `host_id + session`、当前 socket generation 与 textarea 的当前 JavaScript value。
2. 确认捕获 target 仍是当前 target，且捕获 generation 仍是当前 OPEN socket。
3. 使用 `TextEncoder` 编码 `value + "\r"`，作为单个 Binary WebSocket frame 发送；不 trim、不 escape、不 quote、不 Unicode normalize，也不使用 modifier 或 bracketed-paste 包裹。
4. 发送成功才清空该 target 的草稿，并清除 one-shot modifier；locked modifier 保持可见但不会影响 Composer payload。
5. Closed socket、stale generation、编码超限或 `send()` 异常均发送零 frame、保留草稿并显示可访问错误；重连后不自动重放。

现有 PTY frame data 上限为 65,536 bytes，因此 Composer 正文最多允许 65,535 个 UTF-8 bytes，为末尾 CR 保留 1 byte。限制按编码后的 byte length 计算，不能依赖按 UTF-16 code unit 计数的 HTML `maxLength`。空正文合法，仅发送一个 CR。

草稿仅保存在页面内存中的 target-keyed Map；切换 Host/Session 时显示对应草稿，绝不把 A target 的草稿带到同名的 B target。Logout、unmount 与页面刷新清除全部草稿，避免 token/secret 落盘。

### 7. 响应式布局以 visual viewport 和交互能力为准

`touch-action: none` 只作用于需要接管手势的 xterm surface；Overview、Settings、Sidebar、Sheet 和其他滚动区恢复原生 pan/zoom。移动 keybar/Composer 的默认呈现由 coarse pointer/touch capability 与用户偏好决定，而不是只看 CSS breakpoint。

移动端 Dialog/Palette 使用 Bottom Sheet，Sidebar 使用具备 backdrop、focus trap、Esc close、background inert 和 focus restore 的 Drawer。Shell 使用 `VisualViewport.height/offset` 与 safe-area tokens 计算可见布局并合并 fit/resize，避免软键盘遮挡输入行和 resize storm。横屏 Composer 默认压缩为一行，但仍可展开编辑。

### 8. 测试分层验证字节、交互和视觉

Vitest 覆盖 command registry 作用域、listener cleanup、状态映射、Search lifecycle、Clipboard generation guard、Preferences rollback 和 Mobile Composer 精确 Binary frame。Composer 用例必须覆盖前后空格、多行、中文/emoji、空值、65,535/65,536 UTF-8 bytes、modifier bypass、send throw、closed/stale generation 与 target 隔离。

Playwright 在 Chromium 与 mobile WebKit 覆盖 Palette、Search、Context Menu、Zen、重连、同名跨 Host Session、safe-area、orientation、IME/composition、Composer 及 Clipboard。Mobile E2E 必须断言只收到一个 frame，内容精确等于用户文本的 UTF-8 bytes 加末尾 CR。Axe 的 critical 与 serious violation 都必须为零，并为 1440×900、390×844 与 844×390 的主要主题建立截图基线。

构建继续执行现有 entry gzip、xterm gzip 与 total Brotli budget；Search 未使用时不得加载 addon。

## Risks / Trade-offs

- [状态来自既有 projection，部分 Session 无法精确判断 waiting/done] → 使用保守 fallback，不凭 UI 推断不存在的语义；后续协议 change 可补充更精确事实。
- [移动 Composer 可一次发送多行或敏感文本] → 明确显示目标、只由显式发送动作触发、草稿不落盘、不自动重放；保持用户要求的原样语义。
- [发送成功只代表当前 WebSocket 接受 frame，不代表 Shell 执行成功] → 文案使用“已发送”而不是“执行成功”，不做隐式 retry/dedupe。
- [Command Palette 可能继续抢占终端快捷键] → 通过 registry scope 和 PTY byte tests 将应用命令与 Terminal 原生控制键隔离。
- [共享 primitives 引发大范围视觉 diff] → 先覆盖 Cockpit、Palette、Dialog/Sheet 和反馈组件，再逐步迁移现有页面；保留现有主题兼容。
- [SearchAddon 和 primitives 推高 bundle] → 独立动态 chunk、无大型依赖，并以现有 CI budget 作为硬门禁。
- [与未归档 mobile-terminal change 修改相邻代码和测试] → 实现前先同步其已完成能力；新 change 只增加 Workspace/Composer 行为，归档时按实际主规格状态解决 capability 合并。
- [当前 Runtime 没有 kill-session action] → 终止入口采用 capability-gated 呈现；本 change 只实现明确 Detach 语义，不扩展后端或伪造终止成功。

## Migration Plan

1. 先引入 tokens、primitives、command registry 和统一状态 view model，不改变默认 Terminal 数据路径。
2. 迁移 Palette、TopBar/StatusBar、Sidebar 与 Terminal Cockpit，并保留可回退的原导航入口直到 E2E 通过。
3. 动态接入 Search、Context Menu、Clipboard feedback 与 listener cleanup。
4. 在既有 mobile toolbar 上增加 Composer 和 target-keyed memory drafts，完成 byte-level 单元测试后再启用 UI。
5. 完成 desktop/mobile WebKit、Axe、visual regression 与 bundle gate后随嵌入前端发布。

本 change 不涉及持久数据或协议 migration。回滚到上一 release 会同时恢复上一版嵌入前端；不会留下服务端 schema、草稿或数据库状态。

## Open Questions

无阻塞问题。多 Session 网格、只读分享、Session replay、命令历史和完整品牌重做留给独立 change。

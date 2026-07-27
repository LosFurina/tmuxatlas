# Web Interface Quality Specification

## Purpose

TBD - created by archiving change polish-web-terminal-workspace.

## Requirements

### Requirement: Semantic interface tokens and shared primitives
Web UI SHALL 使用语义化 surface、text、focus、motion、control、terminal chrome 与状态 tokens，并 SHALL 为 Button、IconButton、Tooltip、Dialog/Sheet、Menu、StatusPill、Toast、Skeleton、EmptyState 和 Kbd 提供一致的共享交互原语。主题相关颜色、阴影和 focus ring MUST NOT 在页面组件中以固定品牌颜色重复实现。

#### Scenario: User changes an existing theme
- **WHEN** 用户在 Light、Dark、Retro 或其他受支持主题间切换
- **THEN** Workspace chrome、Terminal actions、Dialog、Toast 与状态提示均使用该主题的语义 tokens，且文字和交互状态保持可辨认

#### Scenario: Developer adds an icon-only action
- **WHEN** 页面使用共享 IconButton 呈现一个无可见文字的操作
- **THEN** control 具有可访问名称、统一 focus-visible 状态和至少适用于当前输入方式的触控目标

### Requirement: Terminal-first visual hierarchy
Web UI SHALL 让活动 Terminal 成为 Session 视图的主要视觉区域，常态 chrome 使用安静层级，只有需要处理的异常使用高显著度。界面正文与控制 SHALL 使用可读的 bundled/system UI font stack，Terminal 和必要的技术值 SHALL 使用 monospace；状态 MUST NOT 仅依靠颜色表达。

#### Scenario: Connected session has no pending alert
- **WHEN** 用户打开一个健康且已连接的 Session
- **THEN** Terminal 获得主要空间与对比度，导航、遥测和常态连接信息不会以持续动画或同等高亮争夺注意力

#### Scenario: Session requires attention
- **WHEN** 当前或其他 Session 进入 waiting、error 或 offline 等需要处理状态
- **THEN** UI 使用文字或图标与语义颜色共同表达状态，并把提醒汇总到可发现的上层入口

### Requirement: Responsive viewport and touch behavior
Web UI SHALL 从 320 CSS px 宽度起支持可操作布局，并 MUST 将手势限制限定在 Terminal surface，而不是全局阻止页面滚动或缩放。Shell、Drawer、Sheet、Toolbar 和 Mobile Input Composer SHALL 响应 safe-area、orientation 与 Visual Viewport 变化，且 resize/fit 事件 SHALL 被合并到有界频率。

#### Scenario: User scrolls settings on a touch device
- **WHEN** 用户在移动设备打开 Overview、Settings、Sidebar 或 Sheet 并执行纵向手势
- **THEN** 非 Terminal 内容按浏览器原生方式滚动，不被全局 `touch-action: none` 阻止

#### Scenario: Software keyboard opens in standalone PWA
- **WHEN** 用户聚焦 Terminal 或 Mobile Input Composer 且 Visual Viewport 变小
- **THEN** 当前输入区域、发送按钮和必要 Terminal 行保持可访问，并且不会产生无界 PTY resize storm

#### Scenario: Phone orientation changes
- **WHEN** viewport 在 390×844 portrait 与 844×390 landscape 间切换
- **THEN** 页面不产生意外横向溢出，关键控制不进入 notch、圆角或 home indicator 区域

### Requirement: Accessible overlays and command interactions
Dialog、Sheet、Drawer、Menu、Command Palette 与 Context Menu SHALL 使用适用的 ARIA semantics，管理初始焦点、focus trap、Esc close、background inert 与关闭后的 focus restore。所有功能 SHALL 可由键盘完成，并 SHALL 尊重 `prefers-reduced-motion`。

#### Scenario: Keyboard user opens Command Palette
- **WHEN** 用户通过快捷键打开 Command Palette、选择命令并关闭
- **THEN** 焦点保持在 Palette 内完成操作，关闭后回到原触发位置或活动 Terminal

#### Scenario: Reduced motion is enabled
- **WHEN** 操作系统设置 `prefers-reduced-motion: reduce`
- **THEN** 非必要 pulse、slide 和装饰动画被关闭，同时状态变化仍有非动画表达

#### Scenario: Automated accessibility audit runs
- **WHEN** CI 对主要 desktop 和 mobile views 执行 Axe
- **THEN** critical 与 serious accessibility violations 均为零

### Requirement: Distinct loading, empty, error and operation feedback
Web UI SHALL 明确区分初始 Loading、Ready-but-empty、stale/reconnecting 与 Error 状态。创建、重命名、Clipboard、Preferences 保存和其他异步操作 SHALL 显示 pending 与最终 success/error；乐观更新失败 MUST 回滚或提供显式重试，且错误不得被静默吞掉。

#### Scenario: Initial snapshot is pending
- **WHEN** 用户已进入应用但首个完整 state snapshot 尚未 ready
- **THEN** UI 显示稳定 Skeleton 或 Loading，不显示“No sessions”之类的 ready empty state

#### Scenario: Preferences save fails
- **WHEN** 浏览器中的偏好更新请求返回非成功响应或网络错误
- **THEN** UI 恢复最后确认值或保留明确的未保存状态，并显示可重试错误

#### Scenario: Saved startup preferences are applied
- **WHEN** 已确认的 Preferences 指定 Default View 或 Sidebar Default 且 URL 没有更高优先级的显式目标
- **THEN** 下一次应用初始化采用这些值，并且 Sidebar、App 与 Settings 使用同一已确认偏好来源

#### Scenario: Empty workspace is ready
- **WHEN** 完整 snapshot 已确认且确实不存在 Session
- **THEN** UI 显示带 New Session 或 Setup 动作的 EmptyState

### Requirement: Bounded frontend loading and dependency cost
Terminal、Search 及非首屏页面 SHALL 按实际使用加载。项目 MUST 继续执行版本控制中的 entry gzip、xterm gzip 和 total Brotli budgets；新增组件、addon 或视觉资源超过预算时 MUST 使 CI 失败，除非通过独立变更明确调整预算和理由。

#### Scenario: User never opens a Terminal
- **WHEN** 用户只访问 Login、Overview 或 Settings
- **THEN** 浏览器不加载 xterm runtime

#### Scenario: User never opens Terminal Search
- **WHEN** 用户进入 Terminal 但没有触发 Search
- **THEN** 浏览器不加载 Search addon chunk

#### Scenario: Bundle exceeds configured budget
- **WHEN** 构建产物的任一受控压缩指标超过版本控制阈值
- **THEN** CI 失败并报告具体 asset 和超限值

### Requirement: Visual and interaction regression coverage
项目 SHALL 对主要 Workspace 状态建立稳定的 desktop 与 mobile visual regression，并 SHALL 使用组件测试和浏览器测试覆盖共享 primitives、responsive layout、focus、theme、reduced motion 与异步反馈。

#### Scenario: Visual regression suite runs
- **WHEN** CI 渲染 1440×900、390×844 和 844×390 的代表性 Light、Dark 与 Retro Workspace
- **THEN** 截图与已批准基线比较，未经批准的布局或主题差异使测试失败

#### Scenario: Overlay regression suite runs
- **WHEN** CI 使用键盘依次操作 Dialog、Sheet、Drawer、Menu 和 Command Palette
- **THEN** 每个 overlay 的 focus、关闭、恢复和 accessible name 行为符合共享原语契约

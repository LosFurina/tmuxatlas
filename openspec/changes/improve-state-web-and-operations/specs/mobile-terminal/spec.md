## ADDED Requirements

### Requirement: Touch terminal toolbar
Web UI SHALL 在 touch/coarse-pointer 设备或用户显式启用时提供可收起 Terminal toolbar，至少包含 Esc、Tab、Ctrl、Alt、方向键、Copy、Paste 和软键盘控制。

#### Scenario: User opens a session on mobile
- **WHEN** touch device 显示 Terminal
- **THEN** toolbar 可用且不会被 viewport edge 或 safe-area 遮挡

#### Scenario: User collapses the toolbar
- **WHEN** 用户收起 toolbar
- **THEN** Terminal 获得释放的可见空间，并保留可发现的恢复入口

### Requirement: Deterministic terminal key sequences
Esc、Tab 和方向键 SHALL 发送与物理键一致的 xterm sequence。Ctrl/Alt SHALL 支持清楚标示的 one-shot 或 locked modifier，并与下一输入组合；未使用 modifier MUST NOT 泄漏到另一个 host/session 或 reconnect generation。

#### Scenario: User sends Ctrl-C
- **WHEN** 用户启用 Ctrl 并选择 `C`
- **THEN** 当前 OPEN terminal connection 收到一个 Ctrl-C control sequence，modifier 状态按所选模式清除或保持

#### Scenario: Target changes with a pending modifier
- **WHEN** host/session 切换、连接断开或 Terminal unmount 时 modifier 仍激活
- **THEN** pending modifier 被清除，且不会应用到新目标

### Requirement: Explicit mobile clipboard behavior
Copy SHALL 仅复制当前 xterm selection；Paste SHALL 仅从用户 gesture 读取 clipboard，并把取得的文本发送到读取完成时仍为当前 generation 的 OPEN connection。成功、空内容、权限拒绝和 API 不可用 SHALL 有明确反馈。

#### Scenario: Copy selection succeeds
- **WHEN** 用户选中文本并触发 Copy
- **THEN** selection 被写入 clipboard，UI 提供成功反馈且不向 PTY 发送该文本

#### Scenario: Paste permission is denied
- **WHEN** 浏览器拒绝 clipboard read
- **THEN** UI 显示可恢复错误，不发送空值或之前缓存的 clipboard 文本

#### Scenario: Connection changes during clipboard read
- **WHEN** clipboard Promise 完成前 Terminal 已切换 generation
- **THEN** 读取到的文本不会发送到旧或新 session

### Requirement: Safe-area responsive application layout
Top bar、Terminal/content、toolbar、modal 和 status 区域 SHALL 适配 `env(safe-area-inset-*)`。窄屏 sidebar SHALL 使用可关闭 drawer，modal SHALL 不超过可见 viewport。

#### Scenario: Installed app runs on a notched phone
- **WHEN** PWA 以 portrait 或 landscape standalone mode 打开
- **THEN** 关键控制和 terminal cells 不位于 notch、圆角或 home indicator 下方

#### Scenario: Sidebar opens on a narrow viewport
- **WHEN** 用户在 mobile 打开 session navigation
- **THEN** sidebar 作为 overlay/drawer 出现并可关闭，不永久压缩 Terminal 到不可用宽度

### Requirement: Soft keyboard viewport handling
Web UI SHALL 响应 Visual Viewport 或等价的软键盘可见区域变化，并 coalesce Terminal fit/resize，避免 toolbar、prompt 行或输入区域被键盘遮挡及 resize storm。

#### Scenario: Software keyboard opens
- **WHEN** 用户聚焦 xterm input 且 visual viewport 变小
- **THEN** Terminal 按可见区域重新布局，当前 prompt 与 toolbar 保持可访问

#### Scenario: Viewport emits rapid resize events
- **WHEN** 软键盘动画连续改变 viewport
- **THEN** fit 与 PTY resize 被合并到有界频率，不为每个原始事件创建无界工作

### Requirement: Mobile terminal accessibility and desktop preservation
Toolbar control SHALL 具有 accessible name、可见状态、键盘等价操作和至少 44×44 CSS px 的触控目标。Desktop physical keyboard、xterm selection、scrollback 与 terminal byte semantics SHALL 保持兼容。

#### Scenario: Screen reader explores toolbar
- **WHEN** assistive technology 聚焦 modifier 或 clipboard control
- **THEN** 它能读出 control 名称、当前 pressed/checked 状态和作用

#### Scenario: Desktop user opens Terminal
- **WHEN** 设备使用 physical keyboard 且未显式启用 toolbar
- **THEN** 原有键盘直通、selection 和 scrollback 可用，toolbar 不强制占用空间

### Requirement: Mobile terminal browser verification
项目 SHALL 使用 Chromium 和 mobile WebKit browser tests 覆盖 toolbar sequences、modifier reset、clipboard failure、safe-area、orientation、soft keyboard resize 与可访问性。

#### Scenario: Mobile E2E suite runs
- **WHEN** CI 执行 touch/mobile projects
- **THEN** suite 验证输入只到达当前 host/session，并且 controls 在支持的 viewport 中可见且可操作

# Terminal Workspace Specification

## Purpose

TBD - created by archiving change polish-web-terminal-workspace.

## Requirements

### Requirement: 状态驱动的 Session 导航
Terminal Workspace SHALL 以 stable host ID 和包含 Host identity 的 Session target 组织导航，并展示统一的 `running`、`waiting`、`done`、`error` 与 `offline` 状态。Host SHALL 汇总其 Session 数量和最高优先级待处理状态；用户 SHALL 能按文本与状态搜索或筛选 Session，并能查看最近使用和 Pin 的 Session。Display name MUST NOT 用作唯一身份、操作目标或合并 Host 的依据。

#### Scenario: 两个 Host 存在同名 Session
- **WHEN** 两个不同 stable host ID 都包含名为 `work` 的 Session
- **THEN** Sidebar、搜索结果、最近使用和 Pin 区域分别展示两个原子 target，选择或操作其中一个不会影响另一个

#### Scenario: Host 包含多个待处理状态
- **WHEN** 一个 Host 同时包含 `waiting`、`error` 和正常运行的 Session
- **THEN** Host 行显示可解释的 Session 数量与最高优先级状态，并允许用户筛选到对应 Session

#### Scenario: 用户固定并再次访问 Session
- **WHEN** 用户 Pin 一个 Session、切换到其他视图后再返回 Terminal Workspace
- **THEN** 该 Session 仍位于 Pin 区域，并保持原 stable Host + Session target

### Requirement: Session 生命周期操作必须无歧义
Terminal Workspace SHALL 在导航和菜单中明确区分关闭浏览器 Terminal、detach 当前客户端与真正结束 tmux Session。普通导航、关闭视图或进入 Zen Mode MUST NOT 终止 tmux Session。只有 Runtime 明确暴露受支持的终止 capability 时，UI 才能显示 kill 操作；该操作 MUST 使用明确的破坏性文案、显示完整 Host + Session target，并在提交前要求确认。缺少该 capability 时，UI MUST NOT 显示无法执行或会被错误回退的 kill 操作。

#### Scenario: 用户关闭当前 Terminal 视图
- **WHEN** 用户关闭浏览器中的当前 Terminal 或导航到另一个页面
- **THEN** UI 仅释放或 detach 当前浏览器连接，并且不会发送 kill-session 操作

#### Scenario: 用户请求结束 tmux Session
- **WHEN** Runtime 声明支持终止 capability 且用户选择结束 tmux Session
- **THEN** UI 显示对应 Host 和 Session 的确认信息，并仅在用户明确确认后调用既有终止操作

#### Scenario: Runtime 不支持结束 tmux Session
- **WHEN** 当前 Runtime 没有声明终止 capability
- **THEN** UI 不显示 kill 操作，并明确说明关闭当前浏览器视图只会 detach 而不会停止 tmux Session

### Requirement: 单一 Command Registry
Web UI SHALL 使用一个 command registry 作为应用命令 ID、标签、适用作用域、快捷键、可用条件、Command Palette 条目和 Help 展示的唯一来源。Registry command MUST 明确声明其属于应用级还是 Terminal 级；应用级快捷键 MUST NOT 默认抢占发送给 PTY 的 `Ctrl+H`、`Ctrl+L` 或其他终端控制序列。

#### Scenario: Help 展示快捷键
- **WHEN** 用户打开快捷键 Help
- **THEN** Help 从 command registry 展示当前命令及实际快捷键，而不是维护另一份可能过期的列表

#### Scenario: Terminal 聚焦时输入控制键
- **WHEN** Terminal 获得焦点且用户按下未被显式注册为全局命令的 `Ctrl+H` 或 `Ctrl+L`
- **THEN** 对应按键序列原样进入当前 PTY，应用导航不拦截该输入

#### Scenario: 命令在当前上下文不可用
- **WHEN** command registry 的可用条件判定当前没有 OPEN Terminal target
- **THEN** Command Palette 和其他入口一致地禁用或隐藏依赖该 target 的命令，且不会调用命令 handler

### Requirement: 统一 Command Palette
Quick Switcher SHALL 升级为统一 Command Palette，并 SHALL 支持搜索 Host、Session、Window、Agent、状态以及 command registry 中的应用命令。Palette SHALL 支持键盘打开、分组结果、方向键导航、确认执行与取消后的焦点恢复，并 SHALL 可执行导航、创建 Session、重连、Fullscreen、Overview 和 Settings 等适用命令。Session 或 Window 结果 MUST 携带完整 stable target。

#### Scenario: 搜索跨 Host 的同名 Session
- **WHEN** 用户在 Command Palette 搜索一个存在于多个 Host 的 Session 名称
- **THEN** 每个结果显示可区分的 Host、Session 和状态，并把确认操作路由到所选 stable target

#### Scenario: 通过 Palette 执行应用命令
- **WHEN** 用户搜索并确认 `Fullscreen`、`Reconnect` 或其他已启用的 registry command
- **THEN** Palette 调用同一 command handler、关闭弹层，并把焦点恢复到命令定义的目标

#### Scenario: 用户取消 Palette
- **WHEN** 用户按 Escape 或使用关闭操作取消 Command Palette
- **THEN** 不执行当前高亮结果，并把焦点恢复到打开 Palette 之前的控件或 Terminal

### Requirement: Terminal Cockpit 上下文
Terminal Workspace SHALL 在当前 Terminal 邻近位置提供紧凑的 Terminal Cockpit，显示 Host、Session、Window、Pane、Agent 状态与 PTY 连接状态，并提供 Search、Copy、Paste、字号、scroll-to-bottom、Fullscreen、Zen Mode 和更多操作入口。Cockpit MUST 使用 stable target 数据，且 MUST NOT 转换、包装或解释 Terminal byte stream。

#### Scenario: 用户切换 Window 或 Pane
- **WHEN** 当前 stable target 的 Window 或 Pane 发生变化
- **THEN** Cockpit 更新对应 breadcrumb 和操作目标，且不会把旧 target 的操作发送到新 target

#### Scenario: Terminal 当前没有可用 target
- **WHEN** Session 已结束、Agent offline 或尚未选择 Session
- **THEN** Cockpit 显示明确的不可用原因并禁用需要 OPEN PTY 的操作

### Requirement: 按需加载 Terminal Search
Terminal Workspace SHALL 提供 Terminal scrollback 搜索，并 MUST 在用户首次调用 Search 时才加载 xterm Search addon 或等价搜索实现。搜索 SHALL 支持当前查询、匹配数量、上一个和下一个匹配以及大小写选项；关闭搜索 MUST 保留 Terminal 内容和当前 PTY 连接。搜索实现 MUST NOT 无说明进入首屏 entry bundle 或提高既有 bundle budget。

#### Scenario: 首次打开 Terminal
- **WHEN** 用户进入 Terminal 但未使用 Search
- **THEN** 浏览器不下载独立 Search addon chunk，Terminal 输入和输出正常工作

#### Scenario: 用户首次搜索 scrollback
- **WHEN** 用户通过 Cockpit 或已注册快捷键打开 Search 并输入查询
- **THEN** Search addon 被按需加载，UI 显示匹配数量并允许在匹配之间移动

#### Scenario: 搜索加载失败
- **WHEN** Search addon chunk 无法加载
- **THEN** UI 显示可恢复错误，保留当前 Terminal 和连接，并允许用户重试

### Requirement: 明确的剪贴板与右键操作
Terminal Workspace SHALL 为 Terminal 提供可发现的 Copy、Paste 和右键菜单，右键菜单至少 SHALL 包含适用的 Copy、Paste、Find 与 Select All 操作。Copy SHALL 仅复制当前 xterm selection；Paste SHALL 仅由明确的用户手势读取 Clipboard，并把取得的文本发送到读取完成时仍匹配的 OPEN target 和 connection generation。多行 Paste MUST 在写入前要求明确确认，并 SHALL 在当前 Terminal 启用 bracketed-paste 时使用兼容的 paste sequence。权限拒绝、空 Clipboard、API 不可用或 generation 改变 MUST NOT 发送旧值、空替代值或缓存内容。

#### Scenario: 右键点击已选择的 Terminal 内容
- **WHEN** 用户在存在 xterm selection 时打开 Terminal 右键菜单
- **THEN** UI 显示可用的 Copy 操作，选择 Copy 只写入 Clipboard 而不向 PTY 发送内容

#### Scenario: Paste 读取期间切换 Session
- **WHEN** 用户触发 Paste 后，在 Clipboard Promise 完成前切换了 Host、Session 或 connection generation
- **THEN** 读取到的文本不会发送到旧 target 或新 target，并显示可恢复反馈

#### Scenario: Clipboard 权限被拒绝
- **WHEN** 浏览器拒绝 Clipboard read 或 write
- **THEN** UI 显示明确错误并保持 Terminal 可操作，不发送缓存内容

#### Scenario: 用户确认多行 Paste
- **WHEN** Clipboard 包含多行文本且用户确认发送
- **THEN** 文本只写入确认时仍匹配的 OPEN target 和 generation，并按当前 Terminal 的 bracketed-paste mode 使用兼容输入序列

### Requirement: 可控的 scroll follow
Terminal Workspace SHALL 仅在用户原本位于 scrollback 底部时自动跟随新输出或 resize 后的底部。用户主动向上滚动后，新输出 MUST NOT 抢走当前位置，Cockpit SHALL 显示可发现的新输出或 scroll-to-bottom 控件；用户触发该控件后 SHALL 回到底部并恢复自动跟随。

#### Scenario: 用户正在查看历史输出
- **WHEN** 用户已经向上滚动且当前 PTY 继续产生输出
- **THEN** 当前阅读位置保持稳定，UI 显示有新输出且不会强制滚到底部

#### Scenario: 用户回到底部
- **WHEN** 用户触发 scroll-to-bottom 或手动滚到 buffer 底部
- **THEN** Terminal 显示最新输出并恢复后续自动跟随

### Requirement: 精确且一致的连接状态
Terminal Workspace SHALL 分别建模 Hub state connection、目标 Agent availability 与当前 PTY connection，并从单一前端状态来源生成状态展示。TopBar、Cockpit、Sidebar、Alert 和 Status 区域 MUST NOT 对同一连接同时显示互相矛盾的状态。只有 canonical state 完成 rehydrate 且目标 PTY 已 OPEN 时，当前 Terminal SHALL 标为 ready；认证过期、Hub 断开、Agent offline、Session ended 与 PTY reconnecting SHALL 提供不同原因和适用恢复动作。

#### Scenario: Hub 重连但状态尚未恢复
- **WHEN** Hub WebSocket 已重新打开但 canonical snapshot 尚未完成 rehydrate
- **THEN** UI 保持 `rehydrating` 或等价状态，而不提前显示 ready

#### Scenario: Hub 在线但 Agent 离线
- **WHEN** application state connection 为 ready，而当前 Session 所属 Agent 为 offline
- **THEN** Sidebar 与 Cockpit 一致显示 Agent offline，并且不会把问题误报为 Hub disconnected

#### Scenario: 登录 Session 过期
- **WHEN** Hub 明确返回 auth-required
- **THEN** UI 停止普通网络重试、显示重新登录动作，并保留非敏感的当前导航上下文

### Requirement: 统一操作与异常反馈
Terminal Workspace SHALL 使用一致的 Alert、Toast、Loading、Empty 和 Error 表达操作结果与需要关注的状态。全局 Alert 区 SHALL 汇总数量和最高优先级，当前 target 的错误 SHALL 在 Cockpit 附近提供上下文与恢复动作；成功、失败或仍在进行中的用户操作 MUST NOT 仅通过日志或短暂颜色变化表达。

#### Scenario: 多个 Session 同时需要关注
- **WHEN** 不同 Host 的多个 Session 进入 waiting 或 error
- **THEN** 全局入口显示汇总数量，用户可以展开并导航到每个 stable target

#### Scenario: 当前 PTY 重连失败
- **WHEN** 当前 target 的 PTY reconnect 达到可报告的失败状态
- **THEN** Cockpit 显示失败原因与 Retry 动作，且其他健康 Session 仍可导航和使用

### Requirement: 真正的 Zen Mode
Zen Mode SHALL 最大化当前 Terminal 可见区域，并隐藏 Sidebar、普通 TopBar、StatusBar、Alert chrome 和非必要 Cockpit 控件。Zen Mode MUST 保留可发现且可通过键盘执行的退出路径、关键连接异常提示和 safe-area 保护；进入或退出 Zen Mode MUST NOT 重建、切换或终止当前 PTY connection。

#### Scenario: 用户进入 Zen Mode
- **WHEN** 当前 Terminal 为 OPEN 且用户执行 Zen Mode 命令
- **THEN** 非必要应用 chrome 被隐藏，Terminal 重新 fit 到可用 viewport，而 connection generation 保持不变

#### Scenario: Zen Mode 中连接失败
- **WHEN** 当前 Agent 或 PTY 在 Zen Mode 中断开
- **THEN** UI 显示不遮蔽 Terminal 的关键异常与恢复入口，并仍允许退出 Zen Mode

### Requirement: Mobile Input Composer
Terminal Workspace SHALL 在手机或 coarse-pointer 布局中提供与 Terminal 相邻、可收起的 Mobile Input Composer。Composer SHALL 使用支持中文与 IME 的文本编辑区和明确的“发送”按钮，并 SHALL 在收起后保留可发现的恢复入口。Composer 控件 MUST 具有可访问名称、状态和至少 44×44 CSS px 的主要触控目标。

#### Scenario: 手机端打开 Terminal
- **WHEN** coarse-pointer 设备进入一个 Session
- **THEN** 用户可以展开文本编辑区、使用 IME 完成输入并通过明确按钮发送

#### Scenario: 用户收起 Composer
- **WHEN** 用户收起 Mobile Input Composer
- **THEN** Terminal 获得释放的 viewport 空间，草稿保持不变且恢复入口仍可操作

#### Scenario: IME 尚未完成组合输入
- **WHEN** 文本编辑区仍处于 composition 状态
- **THEN** Composer 不发送未提交的中间文本，并在 composition 完成后使用最终编辑值

### Requirement: Mobile Input Composer 原样发送
用户点击“发送”时，Composer SHALL 取得编辑区的完整当前值，保持其字节语义并在末尾追加且仅追加一次物理 Enter 等价序列 `\r`。Composer SHALL 使用 UTF-8 把 `value + "\r"` 编码为单个 Binary WebSocket frame，并通过现有 PTY input stream 发送；该路径 MUST 绕过 Mobile Terminal 当前的 Ctrl/Alt modifier encoder。Composer MUST NOT trim、Shell parse、quote、escape、拆词、补全、执行历史展开，或自动改写既有换行、Unicode、中文及已提交 IME 结果。空输入 SHALL 发送且仅发送一个 `\r`。为遵守现有 65,536-byte PTY frame 上限，正文 MUST 不超过 65,535 个 UTF-8 bytes；超限输入 MUST 保留并阻止发送。

#### Scenario: 发送包含空格和中文的文本
- **WHEN** 编辑区内容为包含前后空格、中文、引号或 Shell 元字符的文本
- **THEN** PTY 收到完全相同的编辑内容，紧随其后收到一个 `\r`，且 UI 不解释或重写内容

#### Scenario: 发送多行文本
- **WHEN** 编辑区内容已经包含一个或多个换行
- **THEN** 既有换行保持原样，Composer 只在完整内容末尾追加一个 `\r`

#### Scenario: 发送空输入
- **WHEN** 编辑区为空且用户点击“发送”
- **THEN** 当前 OPEN PTY 收到且仅收到一个 `\r`，等价于物理键盘单独按一次 Enter

#### Scenario: Modifier 已激活时发送
- **WHEN** Mobile Terminal 的 Ctrl 或 Alt modifier 处于 one-shot 或 locked 状态且用户发送 Composer 内容
- **THEN** 唯一 Binary frame 仍精确等于 `UTF8(value + "\r")`，不增加 ESC 或控制字符；成功后 one-shot modifier 被清除，locked modifier 保持可见

#### Scenario: 正文位于字节上限
- **WHEN** Composer 正文编码后恰好为 65,535 个 UTF-8 bytes
- **THEN** 当前 OPEN PTY 收到一个总长 65,536 bytes、末尾为 CR 的 Binary frame

#### Scenario: 正文超过字节上限
- **WHEN** Composer 正文编码后超过 65,535 个 UTF-8 bytes
- **THEN** UI 显示可访问的超限错误、保留完整草稿，并向 PTY 发送零个 frame

### Requirement: Target 与 Generation 安全的 Composer 草稿
Mobile Input Composer SHALL 以 stable Host + Session target 隔离草稿，并仅在当前页面内存中保存，不得同步到 Hub、持久化到浏览器存储或进入命令历史。发送动作 SHALL 捕获点击时的完整 target 与 connection generation，并 MUST 在实际写入前再次确认同一 connection 仍为 OPEN；目标切换、断线、重连、unmount 或 generation 改变 MUST NOT 自动发送或迁移草稿。成功发送 SHALL 只清空对应 target 的草稿；失败或取消发送 MUST 保留草稿并提供可恢复反馈。

#### Scenario: 带草稿切换到另一个 Session
- **WHEN** 用户在 Host A 的 Session `work` 留有草稿并切换到 Host B 的 Session `work`
- **THEN** 新 target 不显示或发送旧草稿，返回原 target 时页面内存中的原草稿仍可恢复

#### Scenario: 点击发送后连接 generation 改变
- **WHEN** 用户点击“发送”后、实际 PTY write 前发生断线或重连并产生新 generation
- **THEN** 发送被取消，旧草稿保持不变，并且内容不会写入旧 generation 或新 generation

#### Scenario: 发送成功
- **WHEN** 完整内容和末尾 `\r` 已写入点击时捕获的 OPEN target 与 generation
- **THEN** 仅对应 stable target 的草稿被清空，并显示非阻塞的成功反馈

#### Scenario: 页面被重新加载
- **WHEN** 用户重新加载或关闭当前 Web 页面
- **THEN** Composer 草稿不会从 Hub 或浏览器持久存储中恢复

### Requirement: Terminal Workspace 自动化验证
项目 SHALL 使用前端单元测试、组件测试及 Chromium 与 mobile WebKit E2E 覆盖 stable target 导航、command registry 与快捷键作用域、Command Palette、Cockpit、Search 懒加载、Clipboard/右键、连接状态、Zen Mode 和 Mobile Input Composer。Composer 测试 MUST 覆盖原样文本、Unicode/中文/IME、多行、空输入、末尾单个 `\r`、失败保留草稿、跨 Host 同名 Session 及 connection generation race。

#### Scenario: Terminal Workspace 测试门禁运行
- **WHEN** CI 执行本 capability 的自动化测试
- **THEN** 所有输入仅到达断言的 stable target 与 generation，应用快捷键不污染 PTY byte stream，且 desktop/mobile 的关键工作流均通过

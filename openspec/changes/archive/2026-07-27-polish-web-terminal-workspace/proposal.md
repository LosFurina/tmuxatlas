## Why

TmuxAtlas 已具备多主机 Session、实时状态、移动端工具栏和 PWA 等核心能力，但前端仍以逐项叠加的导航、状态条和弹层呈现，终端上下文、异常反馈与快捷操作缺少统一层级。随着 Hub 管理的 tmux 与 AI Agent Session 增多，用户需要一个以终端为中心、能快速识别待处理状态且在手机上可安全编辑并发送完整输入的 Terminal Workspace。

## What Changes

- 将 Web Shell 收敛为状态驱动的 Terminal Workspace：
  - Sidebar 以稳定 Host identity 分组 Session，支持状态汇总、最近使用、Pin、搜索与筛选，并明确区分 detach 与真正结束 tmux Session。
  - 将 Quick Switcher 升级为统一 Command Palette，可搜索 Host、Session、Window、Agent 与状态，并执行导航、创建、重连、Fullscreen 等应用命令。
  - 建立唯一 command registry，统一快捷键作用域、Help 展示与 PTY 事件隔离，默认保留终端原生 `Ctrl+H`、`Ctrl+L` 等控制键。
- 增加统一 Terminal Cockpit：
  - 显示 Host / Session / Window / Pane 上下文及精确连接状态。
  - 提供懒加载 Search、Copy、Paste、右键菜单、字号、scroll-to-bottom 和真正的 Zen Mode。
  - 合并重复的 Hub、Agent 与 PTY 连接提示，统一 Alert、Toast、Loading、Empty 与 Error 反馈。
- 增加 Mobile Input Composer：
  - 手机端提供与 Terminal 相邻的可收起文本编辑区和明确的“发送”按钮。
  - 点击发送时，将编辑区全部内容保持原样写入点击时的当前 OPEN PTY generation，并在末尾追加一次物理 Enter 等价的 `\r`；不 trim、不解析 Shell、不做 quoting，也不自动改写换行、Unicode、中文或 IME 结果。
  - 草稿按原子 Host + Session target 隔离且仅保存在当前页面内存中；切换目标、断线或重连不得把旧草稿自动发送到新 generation。
  - 发送成功后清空对应草稿；发送失败时保留草稿并显示可恢复反馈。空草稿发送仍等价于单独按一次 Enter。
- 建立轻量 Web UI 质量基线：
  - 提供语义 design tokens 和 Button、IconButton、Tooltip、Dialog/Sheet、StatusPill、Toast、Skeleton、EmptyState、Kbd 等共享原语。
  - 修复全局 `touch-action`、safe-area、Visual Viewport、窄屏弹层、焦点管理、reduced motion 和异步 Preferences 保存反馈。
  - 为 desktop/mobile、Light/Dark/Retro、键盘操作、Clipboard、IME、重连和视觉布局增加自动化回归。
- 保持现有 tmux byte transparency、Hub/Agent protocol、Passkey、PWA network-only 与多主机输入隔离契约；不增加浏览器分屏、Warp Blocks、文件管理器、离线 Terminal 或多人协作。

## Capabilities

### New Capabilities

- `terminal-workspace`: 状态驱动的会话导航、统一 Command Palette 与 command registry、Terminal Cockpit、搜索和剪贴板操作、精确连接反馈、Zen Mode，以及 target/generation-safe 的 Mobile Input Composer。
- `web-interface-quality`: 轻量共享组件与语义 tokens、响应式和 safe-area 行为、焦点与无障碍约束、异步反馈、视觉回归和 bundle budget。

### Modified Capabilities

无。本 change 消费既有多主机 target/state 数据，并建立在进行中的 `mobile-terminal` 能力之上；在该能力尚未归档为主规格前，不创建并行的 `MODIFIED mobile-terminal` delta。

## Impact

- 主要影响 `web/src/App.tsx`、`web/src/components`、`web/src/hooks/useTerminal.ts`、主题与全局样式、前端状态 selector、PWA viewport 表现及 Playwright/Vitest 测试。
- 前端将新增按需加载的 xterm Search addon 或等价能力；不得无说明提高现有 entry、xterm 与 total Brotli budgets，也不引入大型 UI、图标或图表框架。
- Terminal 输入仍通过现有绑定 `host_id + session + connection generation` 的 WebSocket/PTY stream 发送；Mobile Input Composer 不新增后端 API、命令解释器、历史记录或持久化。
- 需要完成 Chromium 与 mobile WebKit 的输入、快捷键、Clipboard、IME、safe-area、重连、无障碍和视觉回归验证。
- 与 `improve-state-web-and-operations` 的剩余 mobile WebKit/可访问性任务存在测试面交集；实现前应先同步或明确复用其已完成的 mobile toolbar 基线，避免重复实现。

# 侧栏 Window 二级导航设计

## 目标

在 TmuxAtlas 左侧 session 列表中为每个 session 提供可展开的 window 二级列表，允许用户直接点击 window 行进行切换，不依赖 Command Palette，也不占用终端页面高度。

## 组件变更

### Sidebar props

添加 `onWindowSelect: (sessionKey: string, windowIndex: number) => void | Promise<void>`。App 层将该回调连接到 `jumpToSession`（现有 `select-window` 调用）。

### 状态

新增 `expandedWindows: Set<string>`，key 为 `${hostId}/${sessionName}`。初始化、搜索过滤、host 折叠均不重置此 set。在窗口删除、session 下线、window 数量变为 0 时不移除 key，由自然渲染跳过。

### renderSession 组件结构

```
<li key={session.key}>
  <!-- 现有 session 行保持不变 -->
  <div class="group ...">
    <button onClick={() => onSessionSelect(session.source)}>...</button>
    <button onClick={toggleExpand}>chevron</button>
    <button onClick={onTogglePin}>pin</button>
  </div>

  <!-- 新增：window 二级列表 -->
  {expanded && !collapsed && session.source.windows.length > 0 && (
    <ul class="ml-4 space-y-1">
      {session.source.windows.map(window => renderWindow(window, session))}
    </ul>
  )}
</li>
```

### renderWindow

每个 window 行：`{window.index} · {window.name}`，字体 11px，次级文本色。active window 使用 `text-primary` 或 `border-l-2 border-primary`。行高约 28px，与现有紧凑风格一致。

点击 window 调用 `onWindowSelect(session.key, window.index)`。不触发 session select（不导航到新 session）。

### 展开/收起

session 行右侧新增 chevron 按钮，仅在 session 有至少一个 window 且侧栏未折叠时显示。图标使用现有 `▸`/`▾` 风格，与 host 折叠保持一致。`aria-expanded` 绑定展开状态，无障碍标签格式：`Show windows for ${session.name}` / `Hide windows for ${session.name}`。

click handler：toggle `expandedWindows` set。handler 不停止事件传播 —— 但需要避免触发 session select。展开/收起时不清除 `selectedSession`，不改变当前终端连接。

## 边界情况

**侧栏折叠/隐藏**：`collapsed` 或 `collapseMode === 'hidden'` 时不展示 window 行、不展示展开按钮。展开状态不清除。

**单窗口 session**：仍展示展开按钮和单个 window 行，保持一致性。

**无窗口 session**（如 session 刚创建尚未上报 windows）：不展示展开按钮、不渲染空列表。

**window 选择失败**：复用现有 `onRuntimeError` prop。`postRuntimeMutation` 抛出的错误通过 `setTimeout` 确保错误提示与 window 选择解耦。

**window 列表在 host 树中的位置**：window 列表渲染在当前 session `<li>` 内、嵌套在 session 行下方、缩进与 host 折叠的子列表一致（`ml-3 pl-1 border-l` 或类似）。不修改 host 分组逻辑。

**active 状态刷新**：`active` 字段来自 `Session.windows[].active`（后端 `list-windows` 上报）。`onWindowSelect` 不更新本地 `active` 标记；等下一次 `/api/sessions` 或 `sessions-changed` 事件推送后再刷新。选择后立即关闭其余 session 的 window 展开**不强制**，只更新当前 session 的 visual active 标记（后端返回后生效）。

**搜索/过滤后的 window 可见性**：搜索框在 session 行可见时，其展开的 window 行必须一并可见。不按 window 名称二次过滤。

## 数据流

```
            /api/sessions (SSE/poll)
                   │
                   ▼
    Session.windows: Window[]
                   │
                   ▼
    renderWindow(window)
        onClick ──► onWindowSelect(sessionKey, windowIndex)
                          │
                          ▼
              /api/session/select-window
                { host_id, session, window }
                          │
                          ▼
               tmux select-window (Agent)
                          │
                          ▼
              sessions-changed notification
                   │
                   ▼
            Session.windows[].active 更新
```

## 无障碍

- window 行 `<button>`，`aria-label="Window {index}: {name} [{active|inactive}]"`。
- 展开按钮 `aria-controls` 指向 window 列表的 `id`。
- 键盘：Tab 到 chevron 后 Enter/Space 展开，Tab 到 window 后 Enter 选择。

## 不做什么

- 不修改 `pkg/`（tmux Agent、control mode、PTY 管理、session 协议）。
- 不修改 `web/src/App.tsx` 的路由或 session 选择逻辑，仅在 `<Sidebar>` 调用处添加 prop。
- 不修改 host 折叠逻辑、pin/recent 功能或右键菜单。
- 不修改 `QuickSwitcher` 或 `TerminalCockpit` 的 window 标签。

## 验证

1. `npm --prefix web test -- --run src/components/Sidebar.test.tsx`：现有 10 个测试通过；新增 4-5 个测试覆盖展开/收起、window 渲染、active 标记、点击切换请求。
2. `npm --prefix web run build`：生产构建通过。
3. 手动验证（按需）：
   - 多窗口 session 能否展开 window 列表
   - 点击非 active window 是否切换
   - 侧栏折叠时 window 列表是否隐藏
   - 搜索过滤后 window 列表是否仍可见
   - 展开状态在 session 刷新后是否保留

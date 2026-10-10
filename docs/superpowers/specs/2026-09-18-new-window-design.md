# Session 内新建 Window 设计

## 目标

在 TmuxAtlas Hub 的侧栏为每个在线 session 提供创建新 window 的能力。用户可以通过 session 行的 "+" 按钮或右键菜单触发，新建的 window 由 tmux 自动命名，创建后 session 自动切换到新窗口。

## 动机

当前 Hub 支持创建 session、在已有 window 之间切换，但不支持在 session 内新建 window。用户必须在终端内手动执行 `Ctrl-b c` 或 `tmux new-window`，这与 Hub 的管理定位不一致。

---

## 范围

- ✅ 在已有 session 内通过 Hub 创建新 window
- ✅ 侧栏 session 行 "+" 按钮入口
- ✅ 右键 context menu "New Window" 入口
- ✅ tmux 自动命名 window
- ✅ 创建后 session 自动切换到新窗口（`new-window` 的默认行为）
- ❌ 删除/重命名 window
- ❌ 自定义 window 名称
- ❌ overview 页面创建 window

---

## 后端数据流

```
POST /api/session/new-window
  { host_id, session }
      │
      ▼
  handleSessionNewWindow (runtime_api.go)
      │
      ▼
  router.Execute(ctx, "new-window", target, {})
      │
      ▼
  ActionRouter → tmux_executor.Execute("new-window", ...)
      │
      ▼
  client.NewWindow(session) → tmux new-window -P -F '#{window_index}' -t <session>
      │
      ▼
  refreshLocalSessionState → client.ListSessions() → StateMgr.UpdateSessions()
      │
      ▼
  前端 refresh() → sidebar window 列表自动更新
  （session 的当前窗口已自动变为新 window，无需额外 select-window）
```

---

## 后端变更

### 1. `pkg/tmux/client.go` — 新增 `NewWindow` 方法

```go
func (c *Client) NewWindow(sessionName string) (int, error) {
    out, err := c.Exec("new-window", "-P", "-F", "#{window_index}", "-t", sessionName)
    if err != nil {
        return 0, err
    }
    index, err := strconv.Atoi(strings.TrimSpace(out))
    if err != nil {
        return 0, fmt.Errorf("parse new window index: %w", err)
    }
    return index, nil
}
```

说明：不使用 `-d`（否则新 window 不会成为当前窗口）。`-P -F '#{window_index}'` 返回新 window index。创建后 tmux 自动将此 window 作为 session 的当前活跃窗口。

### 2. `pkg/peer/tmux_executor.go` — 新增 `"new-window"` case

```go
case "new-window":
    index, err := executor.client.NewWindow(target.Session)
    if err != nil {
        return nil, err
    }
    result, _ := json.Marshal(map[string]any{
        "target": target,
        "window": index,
    })
    return result, nil
```

### 3. `pkg/server/runtime_api.go` — 新增 handler

```go
func handleSessionNewWindow(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor, opts *Options) {
    var request struct {
        HostID  string `json:"host_id"`
        Session string `json:"session"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil || router == nil {
        writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorInvalidTarget})
        return
    }
    result, err := router.Execute(r.Context(), "new-window",
        peer.SessionTarget{HostID: request.HostID, Session: request.Session}, json.RawMessage(`{}`))
    if err != nil {
        writeRuntimeError(w, err)
        return
    }
    refreshLocalSessionState(opts, request.HostID)
    writeActionResponse(w, result)
}
```

### 4. `pkg/server/server.go` — 注册路由

在现有 session mutation 路由组中新增一行：

```go
mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/new-window", func(w http.ResponseWriter, r *http.Request) {
    handleSessionNewWindow(w, r, router, opts)
})
```

路由注册完成后的 session mutation 路由表：

| 方法 | 路径 | Handler |
|------|------|---------|
| POST | /api/session/new | handleSessionNew |
| POST | /api/session/rename | handleSessionRename |
| POST | /api/session/new-window | handleSessionNewWindow |
| POST | /api/session/select-window | handleSessionSelectWindow |
| POST | /api/session/select-pane | handleSessionSelectPane |

---

## 前端数据流

```
Sidebar "+" 或右键 "New Window"
  → onNewWindow(sessionKey)
    → App.handleNewWindow:
        POST /api/session/new-window { host_id, session }
        → await refresh()  // 更新 session 列表（包含新 window）
        → sidebar window 展开自动刷新
        → session 当前窗口已是新 window（终端自动显示）
```

---

## 前端变更

### 5. `web/src/components/Sidebar.tsx`

**新增 prop:**
```typescript
onNewWindow?: (sessionKey: string) => Promise<void>
```

**session 行 "+" 按钮：**

在 pin 按钮左侧、chevron 按钮右侧（即 pin 和 chevron 之间）插入一个 "+" 按钮。仅在以下条件全满足时显示：
- session 在线（`host_online === true`）
- `onNewWindow` prop 已传入
- side 栏未折叠（`!collapsed`）

```tsx
{!collapsed && onNewWindow && (
  <button
    type="button"
    aria-label={`Create new window in ${session.name}`}
    onClick={event => {
      event.stopPropagation()
      void onNewWindow(session.key)
    }}
    title="New window"
    className="mr-0.5 grid h-9 w-8 shrink-0 place-items-center self-center rounded text-muted-foreground opacity-70 hover:bg-muted hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100"
  >
    <span aria-hidden="true">+</span>
  </button>
)}
```

**右键 context menu 新增项：**

在 Rename 项下方插入（第 2 个位置）：

```tsx
{onNewWindow && (
  <button role="menuitem" type="button" onClick={() => {
    void onNewWindow(contextMenu.session.key)
    setContextMenu(null)
  }} className="flex h-10 w-full items-center rounded px-3 text-left hover:bg-accent focus:bg-accent focus:outline-none">
    New Window
  </button>
)}
```

### 6. `web/src/App.tsx`

**新增 callback:**

```typescript
const handleNewWindow = useCallback(async (sessionKey: string) => {
  const { host, name } = parseSessionKey(sessionKey)
  try {
    await postRuntimeMutation('/api/session/new-window', {
      host_id: host, session: name,
    })
    await refresh()
  } catch (err) {
    console.error('Failed to create window:', err)
    setRuntimeError(err instanceof Error ? err.message : 'The session action failed.')
  }
}, [refresh])
```

**传入 Sidebar:**

```tsx
<Sidebar
  onNewWindow={handleNewWindow}
  // ... existing props
/>
```

---

## 验收标准

1. 在线 session 行右侧显示 "+" 按钮，点击后在对应 session 内创建新 window
2. session 右键菜单显示 "New Window"，点击后效果与 "+" 相同
3. 新建 window 后，session 的当前窗口自动切换到新 window
4. sidebar 的 window 子列表在 refresh 后自动反映新 window
5. 离线 session 和侧栏折叠时不显示 "+" 按钮
6. 操作失败时通过 `onRuntimeError` 显示错误信息
7. 现有 13 个 Sidebar 测试继续通过
8. 新增的 NewWindow 测试通过

---

## 不在范围

- 删除 window（`kill-window`）
- 重命名 window（`rename-window`）
- 自定义新 window 名称
- 在 overview 或 command palette 创建 window
- window pane 管理


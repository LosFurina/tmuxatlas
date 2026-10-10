# 终端选择模式切换设计

## 目标

在 Cockpit 工具栏增加选择模式切换按钮，解决 tmux mouse 模式下浏览器端无法选中复制文字的问题。

## 动机

容器内 tmux session 默认启用 `mouse on`（全局 `mouse on` + per-window 继承），xterm.js 检测到 mouse tracking 后将 mousedown/mouseup 作为 SGR 序列转发给 tmux 而非执行本地选择。用户需按住 Shift/Option 才能临时选中文字，但无视觉反馈。

当前解决方式不足：
- 按住 Shift/Option 临时选中：可行但无 UI 提示，新用户不知道这个操作
- 已有側栏 window 切换、"Copy" 按钮可走 Command+C 或右键菜单：但不能覆盖所有场景

本方案在不关闭 tmux mouse 的前提下提供一个持久选择模式开关，兼顾两个需求。

---

## 设计决策

### 不新增快捷键

Shift/Option 本身已是临时选择修饰键。Sel 按钮提供持久锁定就够了，两个机制互补，无需第三个快捷键。

### 按钮实时反映 Shift/Option 状态

当用户按住 Shift/Option 拖拽选字时，Sel 按钮同步亮起作为视觉反馈，松开时恢复。这种「按钮随键盘状态变化」的设计让用户一眼就能看到当前是否处于可选择状态。

---

## 交互设计

### Sel 按钮

位置：Cockpit 工具栏 `Copy` 按钮左侧。

```
[Find] [Sel] [Copy] [Paste] [A−] [A+] [Full] [Zen] [⋯]
```

按钮状态：

| 持久模式 | Shift/Option | 按钮视觉 | 选择行为 |
|---|---|---|---|
| Mouse 模式 | 松开 | 默认（灭） | 必须按住 Shift/Option 拖拽 |
| Mouse 模式 | 按住 | **高亮**（亮） | 原生拖拽，选中后松开保留 |
| 选择模式 | 松开 | **高亮**（锁定态） | 原生拖拽，选中后松开保留 |
| 选择模式 | 按住 | **高亮**（锁定态） | 原生拖拽，无变化 |

按钮行为：
- 点击：切换持久模式（Mouse ↔ 选择模式）
- hover tooltip（Mouse 模式）：`Selection mode`（主行）+ `Hold ⇧ to select text`（副行 muted）
- hover tooltip（选择模式）：`Mouse mode`（主行）+ `Click to restore`（副行）
- `aria-label`：`"Selection mode · Hold Shift to select text"`
  - 选择模式下：`"Mouse mode · Click to restore"`

### 滚轮联动

当 tmux mouse 关闭（前端通过 `term.modes.mouseTrackingMode` 检测）时，`forwardWheelToTmux` 返回 `false`，将滚轮还给 xterm 本地 scrollback（5000 行）。

当 tmux mouse 开启时，保持现有的 SGR wheel 转发行为不变。

### 生命周期

- 切换 session 时：重置为 Mouse 模式（默认）
- 页面刷新后：退回到 Mouse 模式

---

## 后端变更

### `pkg/tmux/client.go` — 新增 `SetMouse`

```go
func (c *Client) SetMouse(sessionName string, on bool) error {
    value := "off"
    if on {
        value = "on"
    }
    _, err := c.Exec("set-option", "-t", sessionName, "mouse", value)
    return err
}
```

### `pkg/peer/tmux_executor.go` — 新增 `"mouse"` case

```go
case "mouse":
    var params struct {
        Value string `json:"value"` // "on" | "off"
    }
    if json.Unmarshal(payload, &params) != nil ||
        (params.Value != "on" && params.Value != "off") {
        return nil, fmt.Errorf("value must be 'on' or 'off'")
    }
    err := executor.client.SetMouse(target.Session, params.Value == "on")
    if err != nil {
        return nil, err
    }
    return json.RawMessage(`{"ok":true}`), nil
```

### `pkg/server/runtime_api.go` — 新增 `handleSessionMouse`

```go
func handleSessionMouse(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor) {
    var request struct {
        HostID  string `json:"host_id"`
        Session string `json:"session"`
        Mouse   string `json:"mouse"` // "on" | "off"
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil || router == nil ||
        (request.Mouse != "on" && request.Mouse != "off") {
        writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorInvalidTarget})
        return
    }
    payload, _ := json.Marshal(map[string]string{"value": request.Mouse})
    result, err := router.Execute(r.Context(), "mouse",
        peer.SessionTarget{HostID: request.HostID, Session: request.Session}, payload)
    if err != nil {
        writeRuntimeError(w, err)
        return
    }
    writeActionResponse(w, result)
}
```

### `pkg/server/server.go` — 注册路由

```go
mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/mouse", func(w http.ResponseWriter, r *http.Request) {
    handleSessionMouse(w, r, actionRouter)
})
```

---

## 前端变更

### `useTerminal.ts` — 滚轮联动

在 `forwardWheelToTmux` 开头增加 mode 检查：

```ts
const forwardWheelToTmux = useCallback((term: Terminal, container: HTMLElement, event: WheelEvent) => {
    // If mouse tracking is off (selection mode), let xterm handle scrolling locally
    if (term.modes.mouseTrackingMode === 'none') return false

    // ... existing forward logic unchanged
}, [...])
```

### `TerminalCockpit.tsx` — 新增 props 和按钮渲染

新增 props：
```ts
selectionMode: boolean           // 是否处于选择模式（持久锁定）
shiftHeld: boolean               // Shift/Option 是否正在按住
onToggleSelectionMode: () => void
```

`CockpitButton` 新增 `highlighted` prop（可选），控制 `data-active` 属性施加 `border-primary text-primary` 样式。

渲染 Sel 按钮在 Copy 按钮左侧：
```tsx
const mouseMode = !selectionMode && !shiftHeld

<CockpitButton
  label={mouseMode
    ? 'Selection mode · Hold Shift to select text'
    : 'Mouse mode · Click to restore'}
  onClick={onToggleSelectionMode}
  highlighted={!mouseMode}
>
  Sel
</CockpitButton>
```

按钮外层用项目已有 `<Tooltip>` 包裹，提供两行 hint：
```tsx
<Tooltip content={<>{mouseMode ? 'Selection mode' : 'Mouse mode'}<br/><span className="text-muted-foreground">{mouseMode ? 'Hold ⇧ to select text' : 'Click to restore'}</span></>}>
  ...button...
</Tooltip>
```

### `Terminal.tsx` — Shift 状态追踪 + selectionMode

新增 state：
```ts
const [selectionMode, setSelectionMode] = useState(false)
const [shiftHeld, setShiftHeld] = useState(false)
```

追踪 Shift/Option/Meta 按下状态：
```ts
useEffect(() => {
    const onKeyEvent = (e: KeyboardEvent) => {
        if (e.key === 'Shift' || e.key === 'Alt' || e.key === 'Meta') {
            setShiftHeld(e.type === 'keydown' ? (e.shiftKey || e.altKey || e.metaKey) : false)
        }
    }
    const onMouseUp = (e: MouseEvent) => {
        if (!e.shiftKey && !e.altKey && !e.metaKey) setShiftHeld(false)
    }
    document.addEventListener('keydown', onKeyEvent)
    document.addEventListener('keyup', onKeyEvent)
    document.addEventListener('mouseup', onMouseUp)
    return () => {
        document.removeEventListener('keydown', onKeyEvent)
        document.removeEventListener('keyup', onKeyEvent)
        document.removeEventListener('mouseup', onMouseUp)
    }
}, [])
```

`onToggleSelectionMode`：
```ts
const onToggleSelectionMode = useCallback(async () => {
    const next = !selectionMode
    setSelectionMode(next)
    try {
        await postRuntimeMutation('/api/session/mouse', {
            host_id: hostId, session: sessionName, mouse: next ? 'off' : 'on',
        })
    } catch (err) {
        console.error('Failed to toggle mouse mode:', err)
        // 回滚前端状态
        setSelectionMode(!next)
    }
}, [hostId, sessionName, selectionMode])
```

切换 session 时重置：
```ts
useEffect(() => {
    setSelectionMode(false)
    setShiftHeld(false)
}, [hostId, sessionName])
```

传递 props 给 `TerminalCockpit`。

### `App.tsx` — 不做额外状态管理

`selectionMode` 和 `shiftHeld` 属于 Terminal 组件内部状态，不影响 TopBar/StatusBar 布局。App 不需要感知。

---

## 测试

- `useTerminal.test.tsx`：`forwardWheelToTmux` 在 `mouseTrackingMode === 'none'` 时返回 `false`，默认模式不变
- `TerminalCockpit.test.tsx`：按钮渲染、highlighted 状态、tooltip 文本正确
- `Terminal.test.tsx`：selectionMode 切换、shiftHeld 追踪、keyup/keydown/mouseup 事件处理

---

## 非目标

- 不持久化 selectionMode 到偏好设置
- 不与现有的 `rightClickSelectsWord` 和 `macOptionClickForcesSelection` 选项冲突
- 不修改 xterm.js 的初始化参数

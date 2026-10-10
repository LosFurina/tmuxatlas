# Session New-Window Implementation Plan

> **For subagent-driven-development**

**Goal:** Add backend and frontend support for creating a new tmux window inside an existing session from the Hub sidebar, with both a "+" button and right-click context menu entry.

**Architecture:** Full-stack change — Go backend adds `new-window` tmux command execution and HTTP endpoint; React frontend adds "+" button on session rows and "New Window" menuitem in the right-click context menu. New window inherits tmux auto-naming; `new-window` without `-d` ensures session auto-switches to the new window.

**Tech Stack:** Go (tmux client, peer executor, HTTP handler), React 19 + TypeScript strict + Vitest + @testing-library/react + Tailwind CSS 4

## Global Constraints

- Follow existing code patterns: new route next to existing session mutation routes, new handler next to existing handlers, new executor case in same switch
- Do not modify `pkg/peer/action_router.go` or `pkg/peer/dispatcher.go` — the executor interface already supports arbitrary operation strings
- Do not modify `pkg/server/runtime_api.go` handler signatures — `handleSessionNewWindow` follows the same `(w, r, router, opts)` pattern as `handleSessionNew` and `handleSessionRename`
- Do not modify routing framework or middleware
- Do not modify window collapse/expand logic, pin/recent, host groups, QuickSwitcher, Overview, or TerminalCockpit
- Session row "+" button appears between pin button and chevron button when: `!collapsed && onNewWindow && host_online`
- Right-click menuitem appears below Rename when `onNewWindow` is provided
- New window creation calls `POST /api/session/new-window` then `refresh()`
- `new-window` tmux command does NOT use `-d` flag — session auto-switches to the new window
- Active window state comes from backend refresh; do not update it optimistically
- `aria-label` on all new interactive elements, `role="menuitem"` on context menu item
- Existing 13 Sidebar tests must continue passing

---

### Task 1: Add `NewWindow` method to tmux Client

**Files:**
- Modify: `pkg/tmux/client.go`

**Interfaces:**
- Consumes: existing `Client.c.Exec` pattern, `strconv`, `strings`, `fmt`
- Produces: `func (c *Client) NewWindow(sessionName string) (int, error)`

Implement `NewWindow` on the `Client` struct that:
- Runs `tmux new-window -P -F '#{window_index}' -t <sessionName>`
- Parses output (trimmed) to int
- Returns the new window index and any error
- No `-d` flag — the new window MUST become the session's current window
- Follow existing error patterns in the same file (e.g., `NewSession`, `SelectWindow`)

- [ ] **Step 1: Add `NewWindow` method**

Place the method after `NewSession` (line ~240) following the same pattern. Add `"fmt"` to imports if not already present.

---

### Task 2: Add `"new-window"` case to tmuxRuntimeExecutor

**Files:**
- Modify: `pkg/peer/tmux_executor.go`

**Interfaces:**
- Consumes: existing `executor.client`, `target.Session`, `json.Marshal`
- Produces: new `case "new-window"` in the switch statement

- [ ] **Step 1: Add `"new-window"` case**

Insert before the `default` case. Call `executor.client.NewWindow(target.Session)`, marshal result with target and window index, return.

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

---

### Task 3: Add HTTP handler and route for new-window

**Files:**
- Modify: `pkg/server/runtime_api.go`
- Modify: `pkg/server/server.go`

**Interfaces:**
- Consumes: existing `runtimeActionExecutor`, `peer.SessionTarget`, `writeRuntimeError`, `refreshLocalSessionState`, `writeActionResponse`, `Options`
- Produces: `handleSessionNewWindow`, route registration

- [ ] **Step 1: Add `handleSessionNewWindow` in `runtime_api.go`**

Add after `handleSessionRename` (after line 53). Follow the exact pattern of `handleSessionNew` — same error handling, same `refreshLocalSessionState` call for local host, same response write. Use `"new-window"` operation, empty payload `json.RawMessage("{}")`.

- [ ] **Step 2: Register route in `server.go`**

Add after the `/session/rename` route (after line 392). One line:

```go
mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/new-window", func(w http.ResponseWriter, r *http.Request) {
    handleSessionNewWindow(w, r, router, opts)
})
```

---

### Task 4: Add tests for new Sidebar elements

**Files:**
- Modify: `web/src/components/Sidebar.test.tsx`

**Interfaces:**
- Consumes: existing `renderSidebar`, `renderStrict`, `Session`, `Window` types, `buildWorkspaceViewModel` helper
- Produces: new test cases appended at end of file

- [ ] **Step 1: Test — "+" button creates new window**

```typescript
it('calls onNewWindow when "+" button is clicked', async () => {
  const onNewWindow = vi.fn().mockResolvedValue(undefined)
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [], new Map(), hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, onNewWindow })

  await userEvent.click(screen.getByRole('button', { name: 'Create new window in work' }))
  expect(onNewWindow).toHaveBeenCalledWith('host-a/work')
})
```

- [ ] **Step 2: Test — right-click menu shows "New Window"**

```typescript
it('shows "New Window" in right-click context menu when onNewWindow provided', async () => {
  const onNewWindow = vi.fn().mockResolvedValue(undefined)
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [], new Map(), hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, onNewWindow })

  const sessionButton = screen.getByRole('button', { name: /Open host-a session work/ })
  fireEvent.contextMenu(sessionButton)
  const menuItem = screen.getByRole('menuitem', { name: 'New Window' })
  expect(menuItem).toBeInTheDocument()
  await userEvent.click(menuItem)
  expect(onNewWindow).toHaveBeenCalledWith('host-a/work')
})
```

- [ ] **Step 3: Test — "+" button hides when collapsed**

```typescript
it('hides "+" button when sidebar is collapsed', () => {
  const onNewWindow = vi.fn().mockResolvedValue(undefined)
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [], new Map(), hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, collapsed: true, onNewWindow })

  expect(screen.queryByRole('button', { name: 'Create new window in work' })).not.toBeInTheDocument()
})
```

- [ ] **Step 4: Test — no "+" button without onNewWindow prop**

```typescript
it('does not show "+" button when onNewWindow prop is omitted', () => {
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [], new Map(), hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws })

  expect(screen.queryByRole('button', { name: 'Create new window in work' })).not.toBeInTheDocument()
})
```

---

### Task 5: Add "+" button and right-click menu item to Sidebar

**Files:**
- Modify: `web/src/components/Sidebar.tsx`

**Interfaces:**
- Consumes: new `onNewWindow?: (sessionKey: string) => Promise<void>` prop, existing `collapsed`, `contextMenu`, `setContextMenu`, `session.host_online`
- Produces: "+" button in session row, "New Window" menuitem in context menu
- Wired: `onWindowSelect` already exists and works; `onNewWindow` is a NEW prop

- [ ] **Step 1: Add `onNewWindow` to SidebarProps interface**

```typescript
onNewWindow?: (sessionKey: string) => Promise<void>
```

- [ ] **Step 2: Destructure from props**

Add `onNewWindow,` to the destructuring list after `onWindowSelect,`.

- [ ] **Step 3: Add "+" button in session row**

Insert between the pin button (`{!collapsed && (...onTogglePin...)}`) and the chevron button (`{!collapsed && session.source.windows.length > 0 && (...)}`). The button:

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

- [ ] **Step 4: Add "New Window" menuitem to context menu**

Insert after the Rename menuitem (after line 492) and before the Pin/Unpin menuitem:

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

---

### Task 6: Wire `handleNewWindow` in App.tsx

**Files:**
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: existing `parseSessionKey`, `postRuntimeMutation`, `refresh`, `setRuntimeError`
- Produces: `handleNewWindow` callback
- Wired: `Sidebar` receives `onNewWindow={handleNewWindow}`

- [ ] **Step 1: Add `handleNewWindow` callback**

Add after `handleCreateSession` (after line 347). Pattern:

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

- [ ] **Step 2: Pass `onNewWindow` to Sidebar**

In the `<Sidebar .../>` JSX, add `onNewWindow={handleNewWindow}` after the `onWindowSelect` prop.

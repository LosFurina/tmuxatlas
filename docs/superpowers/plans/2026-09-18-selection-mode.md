# Selection Mode Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Sel` button in the Cockpit toolbar that toggles tmux mouse mode on/off, enables native text selection in the web terminal, and synchronises scroll behaviour.

**Architecture:** Five-layer change — `tmux.Client.SetMouse` → `tmuxRuntimeExecutor` case → `handleSessionMouse` API handler → `POST /api/session/mouse` route → `Terminal.tsx` state + `TerminalCockpit.tsx` button + `useTerminal.ts` wheel gate. Button state is driven by `selectionMode` (persistent toggle) OR `shiftHeld` (Shift/Option key tracking).

**Tech Stack:** Go 1.x stdlib, xterm.js 5.5.0 `modes.mouseTrackingMode`, React 19, vitest, `@testing-library/react`

## Global Constraints

- Persist no preference across reloads; revert to Mouse mode on session switch
- Do not modify xterm.js constructor options (`rightClickSelectsWord`, `macOptionClickForcesSelection`)
- Do not register a command-palette shortcut
- Tooltip must use the existing `Tooltip` component from `./ui/Tooltip`
- Backend `POST /api/session/mouse` payload keys: `host_id`, `session`, `mouse` (values `"on"` | `"off"`)

---

### Task 1: Backend — `pkg/tmux/client.go` Add `SetMouse`

**Files:**
- Modify: `pkg/tmux/client.go`

**Interfaces:**
- Produces: `func (c *Client) SetMouse(sessionName string, on bool) error`

- [ ] **Step 1: Implement `SetMouse`**

After the existing `CapturePaneContent` method, add:

```go
// SetMouse enables or disables mouse support for the given session.
func (c *Client) SetMouse(sessionName string, on bool) error {
	value := "off"
	if on {
		value = "on"
	}
	_, err := c.Exec("set-option", "-t", sessionName, "mouse", value)
	return err
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/derekchia/dev/workspace/github/tmuxatlas && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add pkg/tmux/client.go
git commit -m "feat: add tmux.Client.SetMouse"
```

---

### Task 2: Backend — `pkg/peer/tmux_executor.go` Add `"mouse"` Case

**Files:**
- Modify: `pkg/peer/tmux_executor.go`

**Interfaces:**
- Consumes: `executor.client.SetMouse(session, bool)` (from Task 1)

- [ ] **Step 1: Implement `"mouse"` case**

In the `Execute` switch, before the `default` case, add:

```go
	case "mouse":
		var params struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(payload, &params) != nil ||
			(params.Value != "on" && params.Value != "off") {
			return nil, fmt.Errorf("value must be 'on' or 'off'")
		}
		if err := executor.client.SetMouse(target.Session, params.Value == "on"); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"ok":true}`), nil
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/derekchia/dev/workspace/github/tmuxatlas && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add pkg/peer/tmux_executor.go
git commit -m "feat: add mouse operation to tmux runtime executor"
```

---

### Task 3: Backend — `pkg/server/runtime_api.go` Add `handleSessionMouse`

**Files:**
- Modify: `pkg/server/runtime_api.go`

**Interfaces:**
- Consumes: `router.Execute(ctx, "mouse", target, payload)` (from Task 2)

- [ ] **Step 1: Add `handleSessionMouse` function**

After `handleSessionNewWindow`, add:

```go
func handleSessionMouse(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor) {
	var request struct {
		HostID  string `json:"host_id"`
		Session string `json:"session"`
		Mouse   string `json:"mouse"`
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

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/derekchia/dev/workspace/github/tmuxatlas && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add pkg/server/runtime_api.go
git commit -m "feat: add handleSessionMouse HTTP handler"
```

---

### Task 4: Backend — `pkg/server/server.go` Register Route

**Files:**
- Modify: `pkg/server/server.go`

**Interfaces:**
- Consumes: `handleSessionMouse` (from Task 3)

- [ ] **Step 1: Register route**

After the existing `session/new-window` route block, add:

```go
mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/mouse", func(w http.ResponseWriter, r *http.Request) {
    handleSessionMouse(w, r, actionRouter)
})
```

Place it right after the `session/new-window` block (around line 402).

- [ ] **Step 2: Verify compilation and check Go tests**

```bash
cd /Users/derekchia/dev/workspace/github/tmuxatlas && go build ./... && go test ./...
```

- [ ] **Step 3: Commit**

```bash
git add pkg/server/server.go
git commit -m "feat: register POST /api/session/mouse route"
```

---

### Task 5: Frontend — `useTerminal.ts` Wheel Gate on `mouseTrackingMode`

**Files:**
- Modify: `web/src/hooks/useTerminal.ts`
- Modify: `web/src/hooks/useTerminal.test.tsx`

**Interfaces:**
- Consumes: `term.modes.mouseTrackingMode` (xterm.js built-in)
- Produces: `forwardWheelToTmux` returns `false` when mouse tracking is off

- [ ] **Step 1: Add failing test for wheel bypass when mouse tracking is off**

In `useTerminal.test.tsx`, add after existing wheel tests:

```ts
  it('bypasses wheel forwarding when mouse tracking mode is off', () => {
    const container = terminalContainer()
    const { result } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => {
      result.current.connect(container)
      FakeWebSocket.sockets[0].open()
    })

    const term = xtermState.terminals[0]
    // Simulate mouse tracking being off
    term.modes = { mouseTrackingMode: 'none' }

    const before = FakeWebSocket.sockets[0].send.mock.calls.length
    act(() => term.emitWheel(-3))
    // No SGR wheel sequence should be sent
    expect(FakeWebSocket.sockets[0].send.mock.calls.length).toBe(before)
  })
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm --prefix web test -- --run web/src/hooks/useTerminal.test.tsx
```
Expected: FAIL — wheel events still forwarded

- [ ] **Step 3: Add gate in `forwardWheelToTmux`**

In `web/src/hooks/useTerminal.ts`, at the top of the `forwardWheelToTmux` callback, add after the first line:

```ts
const forwardWheelToTmux = useCallback((term: Terminal, container: HTMLElement, event: WheelEvent) => {
    if (handledWheelEventsRef.current.has(event)) return false

    // When mouse tracking is off (selection mode), let xterm scroll locally
    if (term.modes.mouseTrackingMode === 'none') return false

    const steps = resolveWheelSteps(term, event)
    // ... rest unchanged
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm --prefix web test -- --run web/src/hooks/useTerminal.test.tsx
```
Expected: PASS — all wheel tests including new one

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useTerminal.ts web/src/hooks/useTerminal.test.tsx
git commit -m "fix: bypass wheel forwarding when tmux mouse tracking is off"
```

---

### Task 6: Frontend — `TerminalCockpit.tsx` Sel Button

**Files:**
- Modify: `web/src/components/TerminalCockpit.tsx`

**Interfaces:**
- Consumes: `selectionMode: boolean`, `shiftHeld: boolean`, `onToggleSelectionMode: () => void` (from Task 7, already defined as props)
- Produces: Renders `Sel` button with `Tooltip`, updates `CockpitButton` to accept `highlighted`

- [ ] **Step 1: Update `CockpitButton` to support `highlighted` prop**

Change `CockpitButton` signature:

```tsx
function CockpitButton({
  label,
  disabled,
  highlighted,
  onClick,
  children,
}: {
  label: string
  disabled?: boolean
  highlighted?: boolean
  onClick: MouseEventHandler<HTMLButtonElement>
  children: ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      data-active={highlighted || undefined}
      onClick={onClick}
      className="min-h-11 shrink-0 rounded border border-border px-2 text-muted-foreground hover:text-foreground disabled:opacity-40 md:min-h-8 data-[active=true]:border-primary data-[active=true]:text-primary"
    >
      {children}
    </button>
  )
}
```

- [ ] **Step 2: Add Sel button with Tooltip to cockpit**

At the top of the file, add the Tooltip import:

```tsx
import { Tooltip } from './ui'
```

Add new props to `TerminalCockpitProps`:

```tsx
interface TerminalCockpitProps {
  // ... existing props ...
  selectionMode: boolean
  shiftHeld: boolean
  onToggleSelectionMode: () => void
}
```

Destructure in function signature:

```tsx
  selectionMode,
  shiftHeld,
  onToggleSelectionMode,
```

Render Sel button between Find and Copy buttons:

```tsx
{/* Replace the existing Find-Sel-Copy buttons area */}
{showScrollToBottom && (
  <CockpitButton label="Scroll to latest Terminal output" onClick={onScrollToBottom}>
    New output ↓
  </CockpitButton>
)}
<CockpitButton label="Search Terminal" onClick={onSearch}>Find</CockpitButton>
<Tooltip content={<>{mouseMode ? 'Selection mode' : 'Mouse mode'}<br /><span className="text-muted-foreground">{mouseMode ? 'Hold \u21e7 to select text' : 'Click to restore'}</span></>}>
  <CockpitButton
    label={mouseMode ? 'Selection mode \u00b7 Hold Shift to select text' : 'Mouse mode \u00b7 Click to restore'}
    highlighted={!mouseMode}
    onClick={onToggleSelectionMode}
  >
    Sel
  </CockpitButton>
</Tooltip>
<CockpitButton label="Copy Terminal selection" disabled={!canCopy} onClick={onCopy}>Copy</CockpitButton>
```

where `const mouseMode = !selectionMode && !shiftHeld` is computed before the return statement.

- [ ] **Step 3: Verify TypeScript compiles**

```bash
npm --prefix web run build
```
Expected: clean build

- [ ] **Step 4: Run existing terminal tests**

```bash
npm --prefix web test -- --run web/src/components/Terminal.test.tsx
```
Expected: all pass (need to update Terminal test to pass `selectionMode={false} shiftHeld={false} onToggleSelectionMode={vi.fn()}`)

- [ ] **Step 5: Commit**

```bash
git add web/src/components/TerminalCockpit.tsx
git commit -m "feat: add Sel button with Tooltip to TerminalCockpit"
```

---

### Task 7: Frontend — `Terminal.tsx` State and Wiring

**Files:**
- Modify: `web/src/components/Terminal.tsx`
- Modify: `web/src/components/Terminal.test.tsx`

**Interfaces:**
- Consumes: `postRuntimeMutation` from `../lib/runtimeApi`, `Tooltip` from `./ui` (already imported by Cockpit)
- Produces: `selectionMode`, `shiftHeld` states; `onToggleSelectionMode` handler; passes to `TerminalCockpit`

- [ ] **Step 1: Add import**

At the top of Terminal.tsx, add:

```tsx
import { postRuntimeMutation } from '../lib/runtimeApi'
```

- [ ] **Step 2: Add state, effects, and handler**

After the existing state declarations (after `const [pasteError, setPasteError] = useState('')`), add:

```tsx
  const [selectionMode, setSelectionMode] = useState(false)
  const [shiftHeld, setShiftHeld] = useState(false)
```

After the existing `useEffect` that resets on session/host change (the one that clears `toolbarError` etc.), add reset:

Inside the existing `useEffect(() => { ... }, [clearSearch, hostId, input, sessionName])`, add:

```tsx
    setSelectionMode(false)
    setShiftHeld(false)
```

Add a new `useEffect` for Shift/Option tracking (after the session-reset effect):

```tsx
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

Add `onToggleSelectionMode` callback after the existing callbacks (near `openSearch`/`closeSearch`):

```tsx
  const onToggleSelectionMode = useCallback(async () => {
    const next = !selectionMode
    setSelectionMode(next)
    try {
      await postRuntimeMutation('/api/session/mouse', {
        host_id: hostId,
        session: sessionName,
        mouse: next ? 'off' : 'on',
      })
    } catch (err) {
      console.error('Failed to toggle mouse mode:', err)
      setSelectionMode(!next)
    }
  }, [hostId, sessionName, selectionMode])
```

- [ ] **Step 3: Pass new props to TerminalCockpit**

In the JSX where `<TerminalCockpit ... />` is rendered, add:

```tsx
        <TerminalCockpit
          hostLabel={hostName || hostId}
          sessionName={sessionName}
          // ... existing props unchanged ...
          selectionMode={selectionMode}
          shiftHeld={shiftHeld}
          onToggleSelectionMode={onToggleSelectionMode}
        />
```

- [ ] **Step 4: Add tests for selection mode toggle**

In `web/src/components/Terminal.test.tsx`, after the existing tests, add:

```ts
describe('selection mode toggle', () => {
  it('renders Sel button in Terminal cockpit', () => {
    render(<Terminal sessionName="one" hostId="host-a" />)
    const sel = screen.getByRole('button', { name: /Selection mode/ })
    expect(sel).toBeVisible()
    expect(sel).toHaveTextContent('Sel')
    expect(sel.dataset.active).toBeUndefined()
  })

  it('highlights Sel button when Shift is held', () => {
    render(<Terminal sessionName="one" hostId="host-a" />)
    const sel = screen.getByRole('button', { name: /Selection mode/ })

    fireEvent.keyDown(document, { key: 'Shift', shiftKey: true })
    expect(sel.dataset.active).toBe('true')

    fireEvent.keyUp(document, { key: 'Shift', shiftKey: false })
    expect(sel.dataset.active).toBeUndefined()
  })

  it('resets selection mode when session changes', () => {
    const view = render(<Terminal sessionName="one" hostId="host-a" />)
    const sel = screen.getByRole('button', { name: /Selection mode/ })
    fireEvent.click(sel)
    expect(sel.dataset.active).toBe('true')

    view.rerender(<Terminal sessionName="two" hostId="host-b" />)
    expect(screen.getByRole('button', { name: /Selection mode/ }).dataset.active).toBeUndefined()
  })
})
```

For the test to work, we need to mock `postRuntimeMutation`:

At the top of the test file, after existing imports, add:

```tsx
import { postRuntimeMutation } from '../lib/runtimeApi'

vi.mock('../lib/runtimeApi', () => ({
  postRuntimeMutation: vi.fn().mockResolvedValue({}),
}))
```

Update global mock to also mock `postRuntimeMutation` and clear it in beforeEach:

```tsx
beforeEach(() => {
  // ... existing ...
  vi.mocked(postRuntimeMutation).mockResolvedValue({})
})
```

- [ ] **Step 5: Run tests**

```bash
npm --prefix web test -- --run web/src/components/Terminal.test.tsx
```
Expected: all tests pass including new selection mode tests

- [ ] **Step 6: Full test suite**

```bash
npm --prefix web test -- --run
```
Expected: all passing

- [ ] **Step 7: Verify production build**

```bash
npm --prefix web run build
```
Expected: clean build

- [ ] **Step 8: Commit**

```bash
git add web/src/components/Terminal.tsx web/src/components/Terminal.test.tsx
git commit -m "feat: wire selection mode toggle through Terminal and tests"
```

---

### Task 8: Integration Verification

**Files:**
- No new files — verify end-to-end

- [ ] **Step 1: Rebuild all Go binaries**

```bash
cd /Users/derekchia/dev/workspace/github/tmuxatlas && go build ./... && go test ./...
```
Expected: build + tests pass

- [ ] **Step 2: Rebuild frontend**

```bash
npm --prefix web run build
```
Expected: clean production build

- [ ] **Step 3: Full test suite**

```bash
npm --prefix web test -- --run && go test ./...
```
Expected: all pass

- [ ] **Step 4: Commit final build artifacts if any**

```bash
git status
```
If `pkg/server/dist/` changed, commit them.

- [ ] **Step 5: Push**

```bash
git push origin main
```

- [ ] **Step 6: Deploy to server and smoke-test**

Build Docker image, deploy, and verify:
1. Open TmuxAtlas, the Sel button appears in cockpit
2. Click Sel → button highlights, text selection works without Shift
3. Hold Shift → button highlights temporarily, text selection works
4. Release Shift → button unhighlights (if not locked)
5. Click Sel again → button unhighlights, mouse mode restored
6. Switch session → Sel resets to unhighlighted

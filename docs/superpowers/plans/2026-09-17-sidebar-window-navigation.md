# Sidebar Window Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an expandable per-session window sub-list in the Sidebar so users can click to switch tmux windows without opening the Command Palette.

**Architecture:** Pure frontend change in `web/src/components/Sidebar.tsx` with a new `onWindowSelect` prop wired from `App.tsx` to the existing `jumpToSession` / `select-window` API. No backend changes. Window list renders inside the session `<li>`, visually indented like host sub-lists, controlled by a per-session `expandedWindows: Set<string>` state that survives search, host collapse, and data refresh.

**Tech Stack:** React 19, TypeScript strict, Vitest + @testing-library/react, Tailwind CSS 4

## Global Constraints

- Do not modify `pkg/` (Go backend, tmux Agent, control mode, PTY, session protocol).
- Do not modify `web/src/App.tsx` routing; only add one prop to `<Sidebar>`.
- Do not modify host collapse logic, pin/recent, right-click menu, QuickSwitcher, or TerminalCockpit.
- Existing 9 Sidebar tests must continue passing.
- Window click calls `onWindowSelect(sessionKey, windowIndex)`, not `onSessionSelect`.
- Active window state comes from backend `Window.active`; do not update it optimistically.
- Chevron icon uses existing `▸`/`▾` pattern matching host collapse.
- `aria-expanded`, `aria-controls`, `aria-label` on all new interactive elements.

---

### Task 1: Write failing tests for window sub-list

**Files:**
- Modify: `web/src/components/Sidebar.test.tsx` (append new test cases after line 212)

**Interfaces:**
- Consumes: `Window` from `../hooks/useSessions`, `Session` (already imported), `renderSidebar` helper (already defined), `renderStrict` from `../test/render`
- Produces: 4 new test cases that will FAIL until Task 2-5 implement the features

Update the test `session()` helper to accept optional `windows`:

```typescript
import type { Window } from '../hooks/useSessions'

function windowsForSession(): Window[] {
  return [
    { id: '@0', session_id: '$1', name: 'editor', index: 0, active: true, layout: '', panes: [] },
    { id: '@1', session_id: '$1', name: 'logs', index: 1, active: false, layout: '', panes: [] },
  ]
}
```

- [ ] **Step 1: Add test — expands and collapses window list on chevron click**

```typescript
it('expands and collapses window sub-list on chevron click', async () => {
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [],
    new Map(),
    hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, selectedSession: 'host-a/work' })

  expect(screen.queryByRole('button', { name: /Window 0:/ })).not.toBeInTheDocument()

  await userEvent.click(screen.getByRole('button', { name: 'Show windows for work' }))
  expect(screen.getByRole('button', { name: /Window 0:/ })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Window 1:/ })).toBeInTheDocument()

  await userEvent.click(screen.getByRole('button', { name: 'Hide windows for work' }))
  expect(screen.queryByRole('button', { name: /Window 0:/ })).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Add test — highlights active window**

```typescript
it('highlights the active window', async () => {
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [],
    new Map(),
    hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, selectedSession: 'host-a/work' })

  await userEvent.click(screen.getByRole('button', { name: 'Show windows for work' }))

  const activeBtn = screen.getByRole('button', { name: 'Window 0: editor, active' })
  expect(activeBtn).toHaveClass('text-primary')

  const inactiveBtn = screen.getByRole('button', { name: 'Window 1: logs, inactive' })
  expect(inactiveBtn).not.toHaveClass('text-primary')
})
```

- [ ] **Step 3: Add test — calls onWindowSelect on window click**

```typescript
it('calls onWindowSelect with session key and window index on window click', async () => {
  const onWindowSelect = vi.fn()
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [],
    new Map(),
    hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, selectedSession: 'host-a/work', onWindowSelect })

  await userEvent.click(screen.getByRole('button', { name: 'Show windows for work' }))
  await userEvent.click(screen.getByRole('button', { name: 'Window 1: logs, inactive' }))

  expect(onWindowSelect).toHaveBeenCalledWith('host-a/work', 1)
})
```

- [ ] **Step 4: Add test — does not show window list when collapsed**

```typescript
it('hides window sub-list when sidebar is collapsed', () => {
  const ws = buildWorkspaceViewModel(
    [{ ...session('host-a', 'work'), windows: windowsForSession() }],
    [],
    new Map(),
    hosts.slice(0, 1),
  )
  renderSidebar({ workspace: ws, collapsed: true })

  // collapsed mode: no chevron, no window rows
  expect(screen.queryByRole('button', { name: 'Show windows for work' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Window 0:/ })).not.toBeInTheDocument()
})
```

- [ ] **Step 5: Run tests to verify they fail**

Run: `npm --prefix web test -- --run src/components/Sidebar.test.tsx`
Expected: existing 9 tests PASS; new 4 tests FAIL (chevron button / window elements not yet rendered)

- [ ] **Step 6: Commit**

```bash
git add web/src/components/Sidebar.test.tsx
git commit -m "test: add failing tests for sidebar window sub-list"
```

---

### Task 2: Add onWindowSelect prop to Sidebar

**Files:**
- Modify: `web/src/components/Sidebar.tsx:10-27` (SidebarProps interface)
- Modify: `web/src/components/Sidebar.tsx:49-63` (destructuring)

**Interfaces:**
- Consumes: none
- Produces: `onWindowSelect?: (sessionKey: string, windowIndex: number) => void | Promise<void>`

- [ ] **Step 1: Add prop type**

In `SidebarProps`, after `onSessionRenamed`:

```typescript
onWindowSelect?: (sessionKey: string, windowIndex: number) => void | Promise<void>
```

- [ ] **Step 2: Destructure prop**

In the function signature destructuring, add `onWindowSelect`:

```typescript
onWindowSelect,
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/Sidebar.tsx
git commit -m "feat: add onWindowSelect prop to Sidebar"
```

---

### Task 3: Add window state and chevron toggle

**Files:**
- Modify: `web/src/components/Sidebar.tsx:64-68` (add state after existing useState blocks)
- Modify: `web/src/components/Sidebar.tsx:249-319` (renderSession — add chevron button)

**Interfaces:**
- Consumes: `onWindowSelect` prop (from Task 2)
- Produces: `expandedWindows: Set<string>`, `toggleWindowExpand` handler, chevron button

- [ ] **Step 1: Add expandedWindows state**

After the existing state declarations (around line 68), add:

```typescript
const [expandedWindows, setExpandedWindows] = useState<Set<string>>(new Set())
```

- [ ] **Step 2: Add chevron button in renderSession**

In `renderSession`, after the pin `<button>` (around line 308), add a chevron button rendered only when `!collapsed` and `session.source.windows.length > 0`:

```typescript
{!collapsed && session.source.windows.length > 0 && (
  <button
    type="button"
    aria-label={expandedWindows.has(session.key) ? `Hide windows for ${session.name}` : `Show windows for ${session.name}`}
    aria-expanded={expandedWindows.has(session.key)}
    onClick={(event) => {
      event.stopPropagation()
      setExpandedWindows(prev => {
        const next = new Set(prev)
        if (next.has(session.key)) next.delete(session.key)
        else next.add(session.key)
        return next
      })
    }}
    className="mr-0.5 grid h-9 w-8 shrink-0 place-items-center self-center rounded text-muted-foreground opacity-70 hover:bg-muted hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100"
  >
    <span aria-hidden="true">{expandedWindows.has(session.key) ? '\u25BE' : '\u25B8'}</span>
  </button>
)}
```

- [ ] **Step 3: Run tests — existing 9 should pass, Task 1 tests should partially pass**

Run: `npm --prefix web test -- --run src/components/Sidebar.test.tsx`
Expected: existing 9 PASS; at least 1 new test should now pass (chevron click + collapse visibility)

- [ ] **Step 4: Commit**

```bash
git add web/src/components/Sidebar.tsx
git commit -m "feat: add window expand/collapse state and chevron toggle"
```

---

### Task 4: Add window sub-list rendering

**Files:**
- Modify: `web/src/components/Sidebar.tsx:249-319` (renderSession — add window list after session row div)

**Interfaces:**
- Consumes: `expandedWindows` state (Task 3), `onWindowSelect` prop (Task 2), `session.source.windows` (existing data)
- Produces: `renderWindow` inline function, window `<ul>` below session row

- [ ] **Step 1: Add window list rendering in renderSession**

After the closing `</div>` of the session row (after the pin button section), add:

```typescript
{expandedWindows.has(session.key) && !collapsed && session.source.windows.length > 0 && (
  <ul className="ml-3 border-l border-sidebar-border pl-1 space-y-0.5">
    {session.source.windows.map(window => (
      <li key={window.id}>
        <button
          type="button"
          aria-label={`Window ${window.index}: ${window.name}, ${window.active ? 'active' : 'inactive'}`}
          onClick={() => onWindowSelect?.(session.key, window.index)}
          className={cn(
            'block w-full rounded px-2 py-0.5 text-left text-[11px] leading-6 hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
            window.active ? 'text-primary font-medium' : 'text-muted-foreground',
          )}
        >
          {window.index}{'\u00B7'} {window.name}
        </button>
      </li>
    ))}
  </ul>
)}
```

- [ ] **Step 2: Import cn if not already available**

`cn` is already imported at line 2. No import changes needed.

- [ ] **Step 3: Run tests — all 13 should pass**

Run: `npm --prefix web test -- --run src/components/Sidebar.test.tsx`
Expected: all 13 PASS (9 existing + 4 new)

- [ ] **Step 4: Commit**

```bash
git add web/src/components/Sidebar.tsx
git commit -m "feat: render window sub-list in sidebar with active highlighting"
```

---

### Task 5: Wire onWindowSelect in App.tsx

**Files:**
- Modify: `web/src/App.tsx:544-566` (Sidebar JSX block)

**Interfaces:**
- Consumes: `jumpToSession` (already defined at line 280)
- Produces: `handleWindowSelect` callback

- [ ] **Step 1: Add onWindowSelect prop to Sidebar invocation**

In App.tsx, inside the `<Sidebar>` JSX block, add after `onRuntimeError={setRuntimeError}`:

```typescript
onWindowSelect={(sessionKey: string, windowIndex: number) => {
  void jumpToSession(sessionKey, windowIndex)
}}
```

- [ ] **Step 2: Run full test suite and verify build**

```bash
npm --prefix web test -- --run src/components/Sidebar.test.tsx
npm --prefix web run build
```

Expected: all 13 tests PASS; build succeeds

- [ ] **Step 3: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat: wire sidebar onWindowSelect to jumpToSession"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run full Sidebar test suite**

```bash
npm --prefix web test -- --run src/components/Sidebar.test.tsx
```

- [ ] **Step 2: Run production build**

```bash
npm --prefix web run build
```

- [ ] **Step 3: Verify git log is clean and consistent**

```bash
git log --oneline -6
```

Expected: 6 commits (design doc, expanded spec, test, prop, state+chevron, window list, App wiring), no unrelated changes.

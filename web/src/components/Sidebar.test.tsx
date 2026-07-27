import { useState, type ComponentProps } from 'react'
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Host } from '../hooks/useHosts'
import type { Session } from '../hooks/useSessions'
import type { ToolEvent } from '../hooks/useToolEvents'
import { postRuntimeMutation } from '../lib/runtimeApi'
import { renderStrict } from '../test/render'
import { buildWorkspaceViewModel } from '../workspace/model'
import { Sidebar } from './Sidebar'

vi.mock('../lib/runtimeApi', () => ({ postRuntimeMutation: vi.fn() }))

function session(host: string): Session {
  return {
    id: `${host}-work`, host, host_name: 'Duplicate Host', host_online: true, name: 'work', created: '', attached: false, last_activity: '2026-01-01T00:00:00Z', windows: [],
  }
}

const sessions = [session('host-a'), session('host-b')]
const hosts: Host[] = [
  { id: 'host-a', name: 'Duplicate Host', online: true, sessions: [], last_seen: '' },
  { id: 'host-b', name: 'Duplicate Host', online: true, sessions: [], last_seen: '' },
]
const events: ToolEvent[] = [
  { host: 'host-a', session: 'work', tool: 'codex', status: 'waiting', window: 0, timestamp: '2026-01-02T00:00:00Z' },
  { host: 'host-b', session: 'work', tool: 'claude', status: 'error', window: 0, timestamp: '2026-01-03T00:00:00Z' },
]
const workspace = buildWorkspaceViewModel(sessions, events, new Map(), hosts)

function renderSidebar(overrides: Partial<ComponentProps<typeof Sidebar>> = {}) {
  const props: ComponentProps<typeof Sidebar> = {
    workspace,
    selectedSession: 'host-a/work',
    collapsed: false,
    collapseMode: 'small',
    pinnedTargets: [],
    recentTargets: [],
    onTogglePin: vi.fn(),
    onSessionSelect: vi.fn(),
    onDetachSession: vi.fn(),
    ...overrides,
  }
  renderStrict(<Sidebar {...props} />)
  return props
}

describe('state-driven Sidebar', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(cleanup)

  it('keeps same-name Sessions on different stable Hosts as separate entries', () => {
    renderSidebar()
    const sessionButtons = screen.getAllByRole('button', { name: /Open Duplicate Host session work/ })
    expect(sessionButtons).toHaveLength(2)
    expect(screen.getByRole('button', { name: /session work, Waiting/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /session work, Error/ })).toBeInTheDocument()
  })

  it('supports text/status filters and Pin actions', async () => {
    const togglePin = vi.fn()
    renderSidebar({ onTogglePin: togglePin })
    await userEvent.type(screen.getByLabelText('Search sessions'), 'codex')
    expect(screen.getAllByRole('button', { name: /Open Duplicate Host session work/ })).toHaveLength(1)
    await userEvent.click(screen.getByRole('button', { name: /Pin Duplicate Host session work/ }))
    expect(togglePin).toHaveBeenCalledWith('host-a/work')

    await userEvent.clear(screen.getByLabelText('Search sessions'))
    await userEvent.selectOptions(screen.getByLabelText('Filter sessions by status'), 'error')
    expect(screen.getAllByRole('button', { name: /Open Duplicate Host session work/ })).toHaveLength(1)
    expect(screen.getByText('claude')).toBeInTheDocument()
  })

  it('promotes canonical pinned and recent targets without merging same-name sessions', () => {
    const calmWorkspace = buildWorkspaceViewModel(sessions, [], new Map(), hosts)
    renderSidebar({ workspace: calmWorkspace, pinnedTargets: ['host-a/work'], recentTargets: ['host-b/work'] })
    expect(screen.getByText('Pinned · 1')).toBeInTheDocument()
    expect(screen.getByText('Recent · 1')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /Open Duplicate Host session work/ })).toHaveLength(2)
  })

  it('makes detach an explicit browser-only action without a runtime mutation', async () => {
    const detach = vi.fn()
    renderSidebar({ onDetachSession: detach })
    const first = screen.getByRole('button', { name: /Open Duplicate Host session work, Waiting/ })
    fireEvent.contextMenu(first, { clientX: 20, clientY: 20 })
    expect(screen.getByRole('menu')).toHaveAccessibleName(/Duplicate Host work actions/)
    await userEvent.click(screen.getByRole('menuitem', { name: 'Detach browser client (tmux keeps running)' }))
    expect(detach).toHaveBeenCalledWith('host-a/work')
    expect(postRuntimeMutation).not.toHaveBeenCalled()
  })

  it('capability-gates destructive kill and confirms the complete Host + Session target', async () => {
    const kill = vi.fn()
    const { unmount } = renderStrict(<Sidebar
      workspace={workspace} selectedSession="host-a/work" collapsed={false} collapseMode="small"
      pinnedTargets={[]} recentTargets={[]} onTogglePin={vi.fn()} onSessionSelect={vi.fn()}
      onDetachSession={vi.fn()}
    />)
    fireEvent.contextMenu(screen.getByRole('button', { name: /Open Duplicate Host session work, Waiting/ }))
    expect(screen.queryByRole('menuitem', { name: 'End tmux session…' })).not.toBeInTheDocument()
    expect(screen.getByText(/Browser detach only/)).toBeInTheDocument()
    unmount()

    renderSidebar({ canKillSession: true, onKillSession: kill })
    fireEvent.contextMenu(screen.getByRole('button', { name: /Open Duplicate Host session work, Waiting/ }))
    await userEvent.click(screen.getByRole('menuitem', { name: 'End tmux session…' }))
    const dialog = screen.getByRole('alertdialog', { name: 'End tmux session permanently?' })
    expect(dialog).toHaveTextContent('Duplicate Host (host-a)')
    expect(dialog).toHaveTextContent('Session: work')
    await userEvent.click(screen.getByRole('button', { name: 'End tmux session' }))
    expect(kill).toHaveBeenCalledWith('host-a/work')
  })

  it('traps the mobile Drawer, makes background inert, closes on Escape and restores focus', async () => {
    function Harness() {
      const [open, setOpen] = useState(true)
      return (
        <div>
          <button type="button" autoFocus data-testid="drawer-trigger">Open sessions</button>
          <Sidebar
            workspace={workspace}
            selectedSession={null}
            collapsed={false}
            collapseMode="small"
            pinnedTargets={[]}
            recentTargets={[]}
            onTogglePin={vi.fn()}
            onSessionSelect={vi.fn()}
            onDetachSession={vi.fn()}
            mobileOpen={open}
            onMobileClose={() => setOpen(false)}
          />
        </div>
      )
    }

    renderStrict(<Harness />)
    const trigger = screen.getByTestId('drawer-trigger')
    await waitFor(() => expect(screen.getByLabelText('Search sessions')).toHaveFocus())
    expect(trigger).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByRole('dialog', { name: 'Workspace sessions' })).toBeInTheDocument()

    fireEvent.keyDown(screen.getByLabelText('Search sessions'), { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Workspace sessions' })).not.toBeInTheDocument())
    expect(trigger).not.toHaveAttribute('aria-hidden')
    await waitFor(() => expect(trigger).toHaveFocus())
  })
})

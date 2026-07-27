import { useState } from 'react'
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { createCommandRegistry } from '../commands/registry'
import type { Host } from '../hooks/useHosts'
import type { Session } from '../hooks/useSessions'
import { renderStrict } from '../test/render'
import { buildWorkspaceViewModel } from '../workspace/model'
import { QuickSwitcher } from './QuickSwitcher'

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
})
afterEach(cleanup)

function session(host: string, hostName: string): Session {
  return {
    id: `${host}-work`, host, host_name: hostName, host_online: true, name: 'work', created: '', attached: false, last_activity: '',
    windows: [{ id: `${host}-window`, session_id: `${host}-work`, name: 'shell', index: 0, active: true, layout: '', panes: [] }],
  }
}

const sessions = [session('host-a', 'Alpha'), session('host-b', 'Beta')]
const hosts: Host[] = [
  { id: 'host-a', name: 'Alpha', online: true, sessions: [], last_seen: '' },
  { id: 'host-b', name: 'Beta', online: true, sessions: [], last_seen: '' },
]
const workspace = buildWorkspaceViewModel(sessions, [], new Map(), hosts)

function commands(overrides: { hasTerminalTarget?: boolean; overview?: () => void } = {}) {
  return createCommandRegistry({
    environment: {
      hasTerminalTarget: overrides.hasTerminalTarget ?? true,
      canReconnect: overrides.hasTerminalTarget ?? true,
      canSignOut: false,
      hasAttention: false,
    },
    handlers: {
      'palette.open': vi.fn(),
      'navigation.overview': overrides.overview || vi.fn(),
      'connection.reconnect': vi.fn(),
    },
  })
}

describe('Command Palette', () => {
  it('exposes dialog, combobox, grouped listbox and stable cross-Host targets', async () => {
    const select = vi.fn()
    renderStrict(<QuickSwitcher workspace={workspace} commands={commands()} pinnedTargets={[]} recentTargets={[]} onSelect={select} onClose={vi.fn()} />)
    expect(screen.getByRole('dialog', { name: 'Command Palette' })).toBeInTheDocument()
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-controls', 'command-palette-results')
    expect(screen.getByRole('listbox')).toBeInTheDocument()

    await userEvent.type(screen.getByRole('combobox'), 'work')
    const results = screen.getAllByRole('option').filter(option => option.textContent?.startsWith('work') && !option.textContent.includes('/ shell'))
    expect(results).toHaveLength(2)
    expect(results.map(option => option.textContent)).toEqual(expect.arrayContaining([expect.stringContaining('Alpha'), expect.stringContaining('Beta')]))
    await userEvent.click(results.find(option => option.textContent?.includes('Beta'))!)
    expect(select).toHaveBeenCalledWith('host-b/work', undefined)
  })

  it('shows disabled commands but never executes them', async () => {
    const reconnect = vi.fn()
    const disabledCommands = createCommandRegistry({
      environment: { hasTerminalTarget: false, canReconnect: false, canSignOut: false, hasAttention: false },
      handlers: { 'palette.open': vi.fn(), 'connection.reconnect': reconnect },
    })
    renderStrict(<QuickSwitcher workspace={workspace} commands={disabledCommands} pinnedTargets={[]} recentTargets={[]} onSelect={vi.fn()} onClose={vi.fn()} />)
    await userEvent.type(screen.getByRole('combobox'), 'Reconnect Terminal')
    const option = screen.getByRole('option', { name: /Reconnect Terminal/ })
    expect(option).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(option)
    expect(reconnect).not.toHaveBeenCalled()
  })

  it('executes keyboard commands and closes without leaking the key to target navigation', async () => {
    const overview = vi.fn()
    const close = vi.fn()
    const select = vi.fn()
    renderStrict(<QuickSwitcher workspace={workspace} commands={commands({ overview })} pinnedTargets={[]} recentTargets={[]} onSelect={select} onClose={close} />)
    const input = screen.getByRole('combobox')
    await userEvent.type(input, 'Go to Overview')
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(close).toHaveBeenCalledTimes(1)
    expect(overview).toHaveBeenCalledTimes(1)
    expect(select).not.toHaveBeenCalled()
  })

  it('restores focus to the trigger after Escape cancellation', async () => {
    function Harness() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>Open palette</button>
          {open && <QuickSwitcher workspace={workspace} commands={commands()} pinnedTargets={[]} recentTargets={[]} onSelect={vi.fn()} onClose={() => setOpen(false)} />}
        </>
      )
    }
    renderStrict(<Harness />)
    const trigger = screen.getByRole('button', { name: 'Open palette' })
    await userEvent.click(trigger)
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus())
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Escape' })
    await waitFor(() => expect(trigger).toHaveFocus())
  })

  it('uses the same ARIA contract in the mobile bottom-sheet shell', () => {
    renderStrict(<QuickSwitcher workspace={workspace} commands={commands()} pinnedTargets={[]} recentTargets={[]} onSelect={vi.fn()} onClose={vi.fn()} />)
    expect(screen.getByRole('dialog')).toHaveClass('command-palette-dialog')
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-expanded', 'true')
  })
})

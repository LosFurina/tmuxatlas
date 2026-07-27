import { describe, expect, it, vi } from 'vitest'
import { createCommandRegistry, dispatchCommandShortcut, getCommand, type ShortcutEventLike } from './registry'

function event(key: string, overrides: Partial<ShortcutEventLike> = {}): ShortcutEventLike {
  return {
    key,
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
    defaultPrevented: false,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
    stopImmediatePropagation: vi.fn(),
    ...overrides,
  }
}

function registry(handlers: Record<string, ReturnType<typeof vi.fn>> = {}) {
  return createCommandRegistry({
    environment: { hasTerminalTarget: true, canReconnect: true, canSignOut: true, hasAttention: true },
    handlers,
    quickSwitcherShortcut: 'ctrl+k',
  })
}

describe('command registry', () => {
  it('keeps stable metadata and evaluates enablement from one source', () => {
    const commands = createCommandRegistry({
      environment: { hasTerminalTarget: false, canReconnect: false, canSignOut: false, hasAttention: false },
      handlers: {
        'palette.open': vi.fn(),
        'terminal.fullscreen': vi.fn(),
        'account.sign-out': vi.fn(),
      },
    })
    expect(getCommand(commands, 'palette.open')).toMatchObject({ label: 'Open Command Palette', category: 'Navigation', scope: 'overlay', enabled: true })
    expect(getCommand(commands, 'terminal.fullscreen')?.enabled).toBe(false)
    expect(getCommand(commands, 'account.sign-out')?.enabled).toBe(false)
  })

  it.each(['h', 'l', 'j'])('leaves unregistered Ctrl+%s untouched for the PTY', key => {
    const run = vi.fn()
    const keyboardEvent = event(key)
    expect(dispatchCommandShortcut(keyboardEvent, registry({ 'palette.open': run }), 'terminal', false)).toBeNull()
    expect(keyboardEvent.preventDefault).not.toHaveBeenCalled()
    expect(keyboardEvent.stopImmediatePropagation).not.toHaveBeenCalled()
    expect(run).not.toHaveBeenCalled()
  })

  it('captures a registered shortcut once before it can reach the PTY', () => {
    const run = vi.fn()
    const keyboardEvent = event('k')
    const command = dispatchCommandShortcut(keyboardEvent, registry({ 'palette.open': run }), 'terminal', false)
    expect(command?.id).toBe('palette.open')
    expect(run).toHaveBeenCalledTimes(1)
    expect(keyboardEvent.preventDefault).toHaveBeenCalledTimes(1)
    expect(keyboardEvent.stopImmediatePropagation).toHaveBeenCalledTimes(1)
  })

  it('honors overlay, workspace and terminal scope priority', () => {
    const fullscreen = vi.fn()
    const commands = registry({ 'terminal.fullscreen': fullscreen })
    const shortcut = () => event('f', { shiftKey: true })
    expect(dispatchCommandShortcut(shortcut(), commands, 'workspace', false)).toBeNull()
    expect(dispatchCommandShortcut(shortcut(), commands, 'overlay', false)).toBeNull()
    expect(dispatchCommandShortcut(shortcut(), commands, 'terminal', false)?.id).toBe('terminal.fullscreen')
    expect(fullscreen).toHaveBeenCalledTimes(1)
  })
})

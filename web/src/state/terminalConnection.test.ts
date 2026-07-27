import { describe, expect, it } from 'vitest'
import { deriveTerminalWorkspaceConnection } from './terminalConnection'

describe('deriveTerminalWorkspaceConnection', () => {
  const connected = {
    hub: 'ready' as const,
    pty: 'connected' as const,
    agentOnline: true,
    sessionAvailable: true,
  }

  it('reports a fully connected target', () => {
    expect(deriveTerminalWorkspaceConnection(connected)).toMatchObject({
      state: 'connected',
      recoverable: false,
    })
  })

  it.each([
    ['connecting', 'connecting'],
    ['rehydrating', 'rehydrating'],
    ['reconnecting', 'hub-offline'],
    ['auth-required', 'auth-required'],
    ['reload-required', 'reload-required'],
  ] as const)('lets Hub state %s take priority', (hub, expected) => {
    expect(deriveTerminalWorkspaceConnection({ ...connected, hub }).state).toBe(expected)
  })

  it('does not expose a stale PTY connection after its Session ends', () => {
    expect(deriveTerminalWorkspaceConnection({
      ...connected,
      sessionAvailable: false,
    }).state).toBe('session-ended')
  })

  it('distinguishes an offline Agent from a PTY reconnect', () => {
    expect(deriveTerminalWorkspaceConnection({
      ...connected,
      agentOnline: false,
    }).state).toBe('agent-offline')

    expect(deriveTerminalWorkspaceConnection({
      ...connected,
      pty: 'reconnecting',
    }).state).toBe('reconnecting')
  })
})

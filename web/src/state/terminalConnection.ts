import type { ConnectionState } from './types'
import type { PtyConnectionState } from '../hooks/useTerminal'

export type TerminalWorkspaceConnectionState =
  | 'connecting'
  | 'rehydrating'
  | 'connected'
  | 'reconnecting'
  | 'hub-offline'
  | 'agent-offline'
  | 'session-ended'
  | 'auth-required'
  | 'reload-required'

export interface TerminalWorkspaceConnectionInput {
  hub: ConnectionState
  pty: PtyConnectionState
  agentOnline: boolean
  sessionAvailable: boolean
}

export interface TerminalWorkspaceConnection {
  state: TerminalWorkspaceConnectionState
  label: string
  detail: string
  recoverable: boolean
}

const connectionCopy: Record<
  TerminalWorkspaceConnectionState,
  Omit<TerminalWorkspaceConnection, 'state'>
> = {
  connecting: {
    label: 'Connecting',
    detail: 'Opening the Terminal connection…',
    recoverable: false,
  },
  rehydrating: {
    label: 'Synchronizing',
    detail: 'Refreshing the current Hub state…',
    recoverable: false,
  },
  connected: {
    label: 'Connected',
    detail: 'Terminal input and output are live.',
    recoverable: false,
  },
  reconnecting: {
    label: 'Reconnecting',
    detail: 'The Terminal connection was interrupted. Retrying…',
    recoverable: true,
  },
  'hub-offline': {
    label: 'Hub offline',
    detail: 'The browser cannot currently reach the Hub.',
    recoverable: true,
  },
  'agent-offline': {
    label: 'Agent offline',
    detail: 'The selected Host is not connected to the Hub.',
    recoverable: true,
  },
  'session-ended': {
    label: 'Session ended',
    detail: 'The selected tmux Session is no longer available.',
    recoverable: false,
  },
  'auth-required': {
    label: 'Sign in required',
    detail: 'Your login session has expired.',
    recoverable: true,
  },
  'reload-required': {
    label: 'Reload required',
    detail: 'TmuxAtlas was updated and this page must be reloaded.',
    recoverable: true,
  },
}

/**
 * Combines the three connection layers used by a Terminal view. Higher-level
 * Hub and target availability always win over the lower-level PTY socket so
 * the UI cannot claim a stale socket is connected after its target vanished.
 */
export function deriveTerminalWorkspaceConnection(
  input: TerminalWorkspaceConnectionInput,
): TerminalWorkspaceConnection {
  let state: TerminalWorkspaceConnectionState

  if (input.hub === 'auth-required') {
    state = 'auth-required'
  } else if (input.hub === 'reload-required') {
    state = 'reload-required'
  } else if (input.hub === 'connecting') {
    state = 'connecting'
  } else if (input.hub === 'rehydrating') {
    state = 'rehydrating'
  } else if (input.hub === 'reconnecting') {
    state = 'hub-offline'
  } else if (!input.sessionAvailable) {
    state = 'session-ended'
  } else if (!input.agentOnline) {
    state = 'agent-offline'
  } else if (input.pty === 'connected') {
    state = 'connected'
  } else if (input.pty === 'connecting') {
    state = 'connecting'
  } else {
    state = 'reconnecting'
  }

  return { state, ...connectionCopy[state] }
}

import type { ConnectionState } from '../state/types'

export function StateConnectionNotice({
  state,
  onAuthRequired,
}: {
  state: ConnectionState
  onAuthRequired?: () => void
}) {
  if (state === 'ready') return null

  const labels: Record<ConnectionState, string> = {
    connecting: 'Connecting to Hub…',
    rehydrating: 'Synchronizing current Hub state…',
    ready: '',
    reconnecting: 'Connection lost. Reconnecting…',
    'auth-required': 'Your login session has expired.',
    'reload-required': 'TmuxAtlas was updated. Reload this page to continue.',
  }
  const action = state === 'reload-required'
    ? () => window.location.reload()
    : state === 'auth-required'
      ? onAuthRequired
      : undefined

  return (
    <div
      role="status"
      aria-live="polite"
      className="absolute top-2 left-1/2 z-[9999] flex min-h-11 -translate-x-1/2 items-center gap-3 rounded border border-border bg-background/95 px-4 py-2 text-sm shadow-lg"
    >
      <span>{labels[state]}</span>
      {action && (
        <button
          type="button"
          onClick={action}
          className="min-h-9 rounded border border-primary/50 px-3 text-primary"
        >
          {state === 'reload-required' ? 'Reload' : 'Sign in'}
        </button>
      )}
    </div>
  )
}

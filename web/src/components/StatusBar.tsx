import type { Host } from '../hooks/useHosts'

interface StatusBarProps {
  sessionCount: number
  waitingCount: number
  pushState: string
  version: string | null
  updateAvailable: boolean
  hosts: Host[]
  agentCount: number
  onHelp?: () => void
}

export function StatusBar({ sessionCount, waitingCount, pushState, version, updateAvailable, hosts, agentCount, onHelp }: StatusBarProps) {
  const peersConnected = hosts.filter(h => !h.local && h.online).length
  const peersConfigured = hosts.filter(h => !h.local).length
  const hostCount = hosts.filter(h => h.online).length

  return (
    <footer className="flex shrink-0 items-center justify-between gap-2 overflow-hidden border-t border-border bg-card px-2 pt-1 pb-[max(.25rem,var(--safe-area-inset-bottom))] font-mono text-xs font-bold text-muted-foreground sm:gap-4 sm:px-4">
      <div className="flex min-w-0 items-center gap-2 sm:gap-4">
        {peersConfigured > 0 && (
          <span className="hidden sm:inline">PEERS: <span className={peersConnected === peersConfigured ? 'text-foreground' : 'text-warning'}>{peersConnected}/{peersConfigured}</span></span>
        )}
        {hosts.length > 1 && (
          <span className="hidden sm:inline">HOSTS: <span className="text-foreground">{hostCount}</span></span>
        )}
        <span className="whitespace-nowrap">SESSIONS: <span className="text-foreground">{sessionCount}</span></span>
        <span className="hidden sm:inline">AGENTS: <span className={agentCount > 0 ? 'text-foreground' : ''}>{agentCount}</span></span>
        {waitingCount > 0 && (
          <span className="whitespace-nowrap text-warning">WAITING: {waitingCount}</span>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2 sm:gap-4">
        {pushState !== 'unsupported' && (
          <span className="hidden items-center gap-1 md:flex">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9" /><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" />
            </svg>
            <span className={pushState === 'subscribed' ? 'text-primary' : pushState === 'denied' ? 'text-destructive' : ''}>
              {pushState === 'subscribed' ? 'PUSH' : pushState === 'denied' ? 'PUSH BLOCKED' : 'PUSH OFF'}
            </span>
          </span>
        )}
        {version && (
          <span className="hidden items-center gap-1 sm:flex">
            {updateAvailable ? (
              <button
                onClick={() => window.location.reload()}
                className="text-warning hover:text-foreground transition-colors"
                title="A new version is available — click to reload"
              >
                {version} (update available)
              </button>
            ) : (
              <span className="text-muted-foreground">{version}</span>
            )}
          </span>
        )}
        {onHelp && (
          <button
            onClick={onHelp}
            aria-label="Keyboard shortcuts"
            className="grid min-h-8 min-w-8 place-items-center rounded text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            title="Keyboard shortcuts (Ctrl+/)"
          >
            ?
          </button>
        )}
      </div>
    </footer>
  )
}

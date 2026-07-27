import type { MouseEventHandler, ReactNode } from 'react'
import type { TerminalWorkspaceConnectionState } from '../state/terminalConnection'

interface TerminalCockpitProps {
  hostLabel: string
  sessionName: string
  windowLabel?: string
  paneLabel?: string
  toolStatus?: string
  connectionState: TerminalWorkspaceConnectionState
  canCopy: boolean
  canPaste: boolean
  showScrollToBottom: boolean
  fullscreen?: boolean
  zenMode?: boolean
  onSearch: () => void
  onCopy: () => void
  onPaste: () => void
  onFontSize: (delta: number) => void
  onScrollToBottom: () => void
  onToggleFullscreen?: () => void
  onToggleZen?: () => void
  onMore: (button: HTMLButtonElement) => void
}

const connectionLabels: Record<TerminalWorkspaceConnectionState, string> = {
  connecting: 'Connecting',
  rehydrating: 'Synchronizing',
  connected: 'Connected',
  reconnecting: 'Reconnecting',
  'hub-offline': 'Hub offline',
  'agent-offline': 'Agent offline',
  'session-ended': 'Session ended',
  'auth-required': 'Sign-in required',
  'reload-required': 'Reload required',
}

export function TerminalCockpit({
  hostLabel,
  sessionName,
  windowLabel,
  paneLabel,
  toolStatus,
  connectionState,
  canCopy,
  canPaste,
  showScrollToBottom,
  fullscreen,
  zenMode,
  onSearch,
  onCopy,
  onPaste,
  onFontSize,
  onScrollToBottom,
  onToggleFullscreen,
  onToggleZen,
  onMore,
}: TerminalCockpitProps) {
  const connectionClass = connectionState === 'connected'
    ? 'text-success'
    : connectionState === 'reconnecting' || connectionState === 'rehydrating'
      ? 'text-warning'
      : connectionState === 'agent-offline' || connectionState === 'session-ended' || connectionState === 'auth-required'
        ? 'text-destructive'
        : 'text-muted-foreground'

  return (
    <div className="terminal-cockpit flex shrink-0 flex-col border-b border-border bg-card font-mono text-xs md:min-h-10 md:flex-row md:items-center md:gap-2 md:px-2">
      <div className="terminal-cockpit__identity flex min-h-9 min-w-0 items-center gap-2 overflow-hidden px-2 md:min-h-0 md:flex-1 md:px-0">
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-hidden whitespace-nowrap text-muted-foreground" aria-label="Terminal target">
          <span className="min-w-0 truncate text-foreground" title={hostLabel}>{hostLabel}</span>
          <span className="shrink-0">/</span>
          <span className="min-w-0 truncate text-foreground" title={sessionName}>{sessionName}</span>
          {windowLabel && <><span className="hidden lg:inline">/</span><span className="hidden lg:inline">{windowLabel}</span></>}
          {paneLabel && <><span className="hidden lg:inline">/</span><span className="hidden lg:inline">{paneLabel}</span></>}
        </div>
        {toolStatus && <span className="hidden shrink-0 rounded border border-border px-1.5 py-0.5 text-muted-foreground lg:inline-flex">{toolStatus}</span>}
        <span role="status" className={`ml-auto shrink-0 ${connectionClass}`}>
          {connectionLabels[connectionState]}
        </span>
      </div>
      <div
        role="toolbar"
        aria-label="Terminal controls"
        className="terminal-cockpit__controls flex min-h-11 w-full shrink-0 items-center gap-1 overflow-x-auto overscroll-x-contain px-2 pb-1 md:min-h-0 md:w-auto md:px-0 md:pb-0"
      >
        {showScrollToBottom && (
          <CockpitButton label="Scroll to latest Terminal output" onClick={onScrollToBottom}>
            New output ↓
          </CockpitButton>
        )}
        <CockpitButton label="Search Terminal" onClick={onSearch}>Find</CockpitButton>
        <CockpitButton label="Copy Terminal selection" disabled={!canCopy} onClick={onCopy}>Copy</CockpitButton>
        <CockpitButton label="Paste into Terminal" disabled={!canPaste} onClick={onPaste}>Paste</CockpitButton>
        <CockpitButton label="Decrease Terminal font size" onClick={() => onFontSize(-1)}>A−</CockpitButton>
        <CockpitButton label="Increase Terminal font size" onClick={() => onFontSize(1)}>A+</CockpitButton>
        {onToggleFullscreen && (
          <CockpitButton
            label={fullscreen ? 'Exit Terminal fullscreen' : 'Enter Terminal fullscreen'}
            onClick={onToggleFullscreen}
          >
            {fullscreen ? 'Window' : 'Full'}
          </CockpitButton>
        )}
        {onToggleZen && (
          <CockpitButton
            label={zenMode ? 'Exit Terminal Zen Mode' : 'Enter Terminal Zen Mode'}
            onClick={onToggleZen}
          >
            {zenMode ? 'Exit Zen' : 'Zen'}
          </CockpitButton>
        )}
        <CockpitButton
          label="More Terminal actions"
          onClick={event => onMore(event.currentTarget)}
        >
          ⋯
        </CockpitButton>
      </div>
    </div>
  )
}

function CockpitButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string
  disabled?: boolean
  onClick: MouseEventHandler<HTMLButtonElement>
  children: ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="min-h-11 shrink-0 rounded border border-border px-2 text-muted-foreground hover:text-foreground disabled:opacity-40 md:min-h-8"
    >
      {children}
    </button>
  )
}

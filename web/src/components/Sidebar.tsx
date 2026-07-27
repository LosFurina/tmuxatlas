import { useEffect, useMemo, useRef, useState } from 'react'
import { cn } from '../lib/utils'
import { postRuntimeMutation } from '../lib/runtimeApi'
import { parseSessionKey, type Session } from '../hooks/useSessions'
import type { WorkspaceSession, WorkspaceStatus, WorkspaceViewModel } from '../workspace/model'
import { filterWorkspaceSessions, isAttentionStatus, workspaceStatusLabel } from '../workspace/model'
import { Button, Dialog } from './ui'
import { focusableElements, makeOutsideInert } from './ui/overlayAccessibility'

interface SidebarProps {
  workspace: WorkspaceViewModel
  selectedSession: string | null
  collapsed: boolean
  collapseMode: 'small' | 'hidden'
  pinnedTargets: string[]
  recentTargets: string[]
  onTogglePin: (target: string) => void
  onSessionSelect: (session: Session) => void
  onDetachSession: (target: string) => void
  canKillSession?: boolean
  onKillSession?: (target: string) => void | Promise<void>
  onSessionRenamed?: (oldKey: string, newKey: string) => void
  onRuntimeError?: (message: string) => void
  mobileOpen?: boolean
  onMobileClose?: () => void
}

const statuses: Array<WorkspaceStatus | 'all'> = ['all', 'running', 'waiting', 'done', 'error', 'offline']

const statusClass: Record<WorkspaceStatus, string> = {
  running: 'bg-success/15 text-success border-success/35',
  waiting: 'bg-warning/15 text-warning border-warning/35',
  done: 'bg-muted text-muted-foreground border-border',
  error: 'bg-destructive/10 text-destructive border-destructive/35',
  offline: 'bg-muted text-muted-foreground border-border',
}

function Sparkline({ data }: { data: number[] }) {
  if (!data.length) return null
  const max = Math.max(...data, 1)
  return (
    <svg viewBox={`0 0 ${data.length} 14`} preserveAspectRatio="none" width="100%" height="14" aria-hidden="true" className="block">
      {data.map((value, index) => (
        <rect key={index} x={index} y={14 - (value / max) * 14} width="0.92" height={(value / max) * 14} className={value > 0 ? 'fill-chart-primary opacity-70' : 'fill-muted opacity-30'} />
      ))}
    </svg>
  )
}
function relativeActivity(timestamp: string | null): string {
  if (!timestamp) return 'No recent activity'
  const elapsed = Date.now() - Date.parse(timestamp)
  if (!Number.isFinite(elapsed) || elapsed < 0) return 'Recently active'
  if (elapsed < 60_000) return 'Active now'
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)}m ago`
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)}h ago`
  return `${Math.floor(elapsed / 86_400_000)}d ago`
}

export function Sidebar({
  workspace,
  selectedSession,
  collapsed,
  collapseMode,
  pinnedTargets,
  onTogglePin,
  onSessionSelect,
  onDetachSession,
  canKillSession = false,
  onKillSession,
  onSessionRenamed,
  onRuntimeError,
  mobileOpen = false,
  onMobileClose,
}: SidebarProps) {
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<WorkspaceStatus | 'all'>('all')
  const [expandedHosts, setExpandedHosts] = useState<Set<string>>(
    () => new Set(workspace.hosts.map(host => host.id)),
  )
  const knownHostsRef = useRef(new Set(workspace.hosts.map(host => host.id)))
  const [renaming, setRenaming] = useState<WorkspaceSession | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [contextMenu, setContextMenu] = useState<{ session: WorkspaceSession; x: number; y: number } | null>(null)
  const [killTarget, setKillTarget] = useState<WorkspaceSession | null>(null)
  const renameInputRef = useRef<HTMLInputElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const asideRef = useRef<HTMLElement>(null)
  const onMobileCloseRef = useRef(onMobileClose)
  const [isMobileViewport, setIsMobileViewport] = useState(() => (
    typeof window.matchMedia !== 'function'
      ? mobileOpen
      : window.matchMedia('(max-width: 1023px)').matches
  ))
  const mobileDrawerActive = mobileOpen && isMobileViewport

  useEffect(() => {
    onMobileCloseRef.current = onMobileClose
  }, [onMobileClose])

  const filtered = useMemo(() => filterWorkspaceSessions(workspace.sessions, query, statusFilter), [query, statusFilter, workspace.sessions])
  const pinnedSet = useMemo(() => new Set(pinnedTargets), [pinnedTargets])
  const filtering = query.trim().length > 0 || statusFilter !== 'all'

  useEffect(() => {
    const currentHostIds = new Set(workspace.hosts.map(host => host.id))
    const addedHostIds = [...currentHostIds].filter(id => !knownHostsRef.current.has(id))
    knownHostsRef.current = currentHostIds
    if (addedHostIds.length === 0) return
    setExpandedHosts(current => new Set([...current, ...addedHostIds]))
  }, [workspace.hosts])

  useEffect(() => {
    if (!selectedSession) return
    const selectedHostId = parseSessionKey(selectedSession).host
    setExpandedHosts(current => {
      if (current.has(selectedHostId)) return current
      return new Set([...current, selectedHostId])
    })
  }, [selectedSession])

  useEffect(() => {
    if (!renaming) return
    renameInputRef.current?.focus()
    renameInputRef.current?.select()
  }, [renaming])

  useEffect(() => {
    if (!contextMenu) return
    const close = (event: Event) => {
      if (event.target instanceof Node && menuRef.current?.contains(event.target)) return
      setContextMenu(null)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setContextMenu(null)
    }
    window.addEventListener('pointerdown', close, true)
    window.addEventListener('keydown', escape, true)
    requestAnimationFrame(() => menuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus())
    return () => {
      window.removeEventListener('pointerdown', close, true)
      window.removeEventListener('keydown', escape, true)
    }
  }, [contextMenu])

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const media = window.matchMedia('(max-width: 1023px)')
    const update = () => setIsMobileViewport(media.matches)
    update()
    if (typeof media.addEventListener === 'function') {
      media.addEventListener('change', update)
      return () => media.removeEventListener('change', update)
    }
    media.addListener?.(update)
    return () => media.removeListener?.(update)
  }, [])

  useEffect(() => {
    if (mobileOpen && !isMobileViewport) onMobileCloseRef.current?.()
  }, [isMobileViewport, mobileOpen])

  useEffect(() => {
    const drawer = asideRef.current
    if (!drawer) return
    drawer.inert = isMobileViewport && !mobileDrawerActive
    return () => {
      drawer.inert = false
    }
  }, [isMobileViewport, mobileDrawerActive])

  useEffect(() => {
    if (!mobileDrawerActive || !onMobileCloseRef.current || !asideRef.current) return
    const drawer = asideRef.current
    const previousFocus = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null
    const restoreBackground = makeOutsideInert(
      drawer,
      element => element.hasAttribute('data-sidebar-backdrop'),
    )
    const focusable = () => focusableElements(drawer)
    const frame = window.requestAnimationFrame(() => (focusable()[0] ?? drawer).focus())
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        onMobileCloseRef.current?.()
        return
      }
      if (event.key !== 'Tab') return
      const targets = focusable()
      if (targets.length === 0) {
        event.preventDefault()
        drawer.focus()
        return
      }
      const first = targets[0]
      const last = targets[targets.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    const onDocumentKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || drawer.contains(event.target as Node)) return
      event.preventDefault()
      onMobileCloseRef.current?.()
    }
    drawer.addEventListener('keydown', onKeyDown)
    document.addEventListener('keydown', onDocumentKeyDown)

    return () => {
      window.cancelAnimationFrame(frame)
      drawer.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('keydown', onDocumentKeyDown)
      restoreBackground()
      window.requestAnimationFrame(() => {
        if (previousFocus?.isConnected) previousFocus.focus()
      })
    }
  }, [mobileDrawerActive])

  const openMenu = (session: WorkspaceSession, x: number, y: number) => setContextMenu({ session, x, y })

  const startRename = (session: WorkspaceSession) => {
    setRenaming(session)
    setRenameValue(session.name)
    setContextMenu(null)
  }

  const submitRename = async () => {
    const current = renaming
    const nextName = renameValue.trim()
    if (!current || !nextName || nextName === current.name) {
      setRenaming(null)
      return
    }
    try {
      await postRuntimeMutation('/api/session/rename', {
        host_id: current.hostId,
        session: current.name,
        new_name: nextName,
      })
      onSessionRenamed?.(current.key, `${current.hostId}/${nextName}`)
    } catch (error) {
      onRuntimeError?.(error instanceof Error ? error.message : 'The session action failed.')
    } finally {
      setRenaming(null)
    }
  }

  const runDetach = (target: string) => {
    setContextMenu(null)
    onDetachSession(target)
  }

  const renderSession = (session: WorkspaceSession, showHost = false) => {
    const selected = selectedSession === session.key
    const isRenaming = renaming?.key === session.key
    return (
      <li key={session.key}>
        <div className={cn(
          'group relative flex w-full rounded-lg border-l-2 transition-colors',
          selected ? 'border-primary bg-sidebar-accent text-sidebar-primary' : isAttentionStatus(session.status) ? 'border-warning bg-warning/5' : 'border-transparent hover:bg-sidebar-accent',
        )}>
          <button
            type="button"
            aria-label={`Open ${session.hostName} session ${session.name}, ${workspaceStatusLabel(session.status)}`}
            aria-haspopup="menu"
            onClick={() => !isRenaming && onSessionSelect(session.source)}
            onContextMenu={event => {
              event.preventDefault()
              openMenu(session, event.clientX, event.clientY)
            }}
            onKeyDown={event => {
              if (event.key === 'F10' && event.shiftKey) {
                event.preventDefault()
                const rect = event.currentTarget.getBoundingClientRect()
                openMenu(session, rect.left + 24, rect.top + 24)
              }
            }}
            className="min-w-0 flex-1 px-3 py-2 text-left text-sidebar-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
          >
            <div className="flex min-w-0 items-start">
              {isRenaming ? (
                <input
                  ref={renameInputRef}
                  aria-label={`Rename ${session.name}`}
                  value={renameValue}
                  onChange={event => setRenameValue(event.target.value)}
                  onKeyDown={event => {
                    event.stopPropagation()
                    if (event.key === 'Enter') void submitRename()
                    if (event.key === 'Escape') setRenaming(null)
                  }}
                  onBlur={() => void submitRename()}
                  onClick={event => event.stopPropagation()}
                  className="min-w-0 flex-1 rounded border border-primary bg-input px-1 py-0.5 font-mono text-sm text-foreground outline-none"
                />
              ) : (
                <span className={cn(
                  'min-w-0 flex-1 font-medium',
                  collapsed ? 'truncate' : 'whitespace-normal break-words leading-5 [overflow-wrap:anywhere]',
                )}>
                  {collapsed ? session.name.charAt(0).toUpperCase() : session.name}
                </span>
              )}
            </div>
            {!collapsed && (
              <>
                <div className="mt-1 flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground">
                  {showHost && <span className="truncate">{session.hostName}</span>}
                  <span className="shrink-0">{relativeActivity(session.lastActivity)}</span>
                  {session.agents.length > 0 && <span className="min-w-0 truncate">{session.agents.join(', ')}</span>}
                  <span className={cn('ml-auto shrink-0 rounded-full border px-1.5 py-0.5 text-[10px] font-medium', statusClass[session.status])}>
                    {workspaceStatusLabel(session.status)}
                  </span>
                </div>
                {session.activity?.sparkline?.length ? <div className="mt-1"><Sparkline data={session.activity.sparkline} /></div> : null}
              </>
            )}
          </button>
          {!collapsed && (
            <button
              type="button"
              aria-label={pinnedSet.has(session.key) ? `Unpin ${session.hostName} session ${session.name}` : `Pin ${session.hostName} session ${session.name}`}
              aria-pressed={pinnedSet.has(session.key)}
              onClick={() => onTogglePin(session.key)}
              className="mr-1 grid h-11 w-9 shrink-0 place-items-center self-center rounded text-muted-foreground opacity-70 hover:bg-muted hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100"
            >
              <span aria-hidden="true">{pinnedSet.has(session.key) ? '★' : '☆'}</span>
            </button>
          )}
        </div>
      </li>
    )
  }

  const filteredKeys = new Set(filtered.map(session => session.key))
  const hostGroups = workspace.hosts.flatMap(host => {
    const hostSessions = host.sessions.filter(session => filteredKeys.has(session.key))
    if (!hostSessions.length) return []
    return [{ host, sessions: hostSessions }]
  })

  const toggleHost = (hostId: string) => {
    setExpandedHosts(current => {
      const next = new Set(current)
      if (next.has(hostId)) next.delete(hostId)
      else next.add(hostId)
      return next
    })
  }

  const isHidden = collapsed && collapseMode === 'hidden'
  return (
    <>
      {mobileDrawerActive && (
        <button
          type="button"
          data-sidebar-backdrop=""
          aria-label="Close session drawer"
          tabIndex={-1}
          className="mobile-sidebar-backdrop z-30 bg-black/50"
          onClick={onMobileClose}
        />
      )}
      <aside
        ref={asideRef}
        role={mobileDrawerActive ? 'dialog' : undefined}
        aria-modal={mobileDrawerActive || undefined}
        aria-hidden={isMobileViewport && !mobileDrawerActive ? true : undefined}
        aria-label="Workspace sessions"
        tabIndex={-1}
        className={cn(
        'mobile-sidebar-drawer z-40 flex w-72 max-w-[85vw] flex-col bg-sidebar text-sm text-sidebar-foreground transition-transform duration-300 lg:z-auto lg:max-w-none lg:transition-all',
        mobileDrawerActive ? 'translate-x-0' : '-translate-x-full pointer-events-none',
        'lg:pointer-events-auto lg:translate-x-0',
        collapsed ? collapseMode === 'hidden' ? 'lg:w-0 lg:overflow-hidden' : 'lg:w-16' : 'lg:w-64',
        !isHidden && 'border-r border-sidebar-border',
      )}>
        {!collapsed && (
          <div className="space-y-2 border-b border-sidebar-border p-3">
            <label className="sr-only" htmlFor="workspace-session-search">Search sessions</label>
            <input
              id="workspace-session-search"
              type="search"
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder="Search host, session, agent…"
              className="h-10 w-full rounded-md border border-border bg-input px-3 text-sm text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
            />
            <label className="sr-only" htmlFor="workspace-status-filter">Filter sessions by status</label>
            <select
              id="workspace-status-filter"
              value={statusFilter}
              onChange={event => setStatusFilter(event.target.value as WorkspaceStatus | 'all')}
              className="h-10 w-full rounded-md border border-border bg-input px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {statuses.map(status => <option key={status} value={status}>{status === 'all' ? 'All statuses' : workspaceStatusLabel(status)}</option>)}
            </select>
          </div>
        )}

        <nav aria-label="Host and session navigation" className="flex-1 overflow-y-auto overscroll-contain p-2">
          {filtered.length === 0 ? (
            <div className="p-4 text-center text-sm text-muted-foreground">{workspace.sessions.length === 0 ? 'No sessions' : 'No sessions match this filter'}</div>
          ) : collapsed ? (
            <ul className="space-y-1">{filtered.map(session => renderSession(session))}</ul>
          ) : (
            <ul className="space-y-1">
              {hostGroups.map(({ host, sessions }) => (
                <li key={host.id}>
                  <button
                    type="button"
                    aria-label={`${filtering || expandedHosts.has(host.id) ? 'Collapse' : 'Expand'} ${host.name} (${host.id}) host sessions`}
                    aria-expanded={filtering || expandedHosts.has(host.id)}
                    onClick={() => toggleHost(host.id)}
                    className="mt-1 flex min-h-10 w-full items-center gap-2 rounded px-3 text-left text-xs font-semibold text-sidebar-foreground hover:bg-sidebar-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                  >
                    <span aria-hidden="true" className="w-3 text-muted-foreground">
                      {filtering || expandedHosts.has(host.id) ? '▾' : '▸'}
                    </span>
                    <span className={cn('h-2 w-2 shrink-0 rounded-full', host.online ? 'bg-success' : 'bg-muted-foreground')} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{host.name}</span>
                      {host.name !== host.id && (
                        <span className="block truncate font-mono text-[10px] font-normal text-muted-foreground">{host.id}</span>
                      )}
                    </span>
                    <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{sessions.length}</span>
                    {sessions.some(session => isAttentionStatus(session.status)) && (
                      <span className="shrink-0 rounded-full bg-warning/15 px-1.5 py-0.5 text-[10px] text-warning">
                        {sessions.filter(session => isAttentionStatus(session.status)).length}
                      </span>
                    )}
                  </button>
                  {(filtering || expandedHosts.has(host.id)) && (
                    <ul className="ml-3 space-y-1 border-l border-sidebar-border pl-1">
                      {sessions.map(session => renderSession(session))}
                    </ul>
                  )}
                </li>
              ))}
            </ul>
          )}
        </nav>

        {contextMenu && (
          <div
            ref={menuRef}
            role="menu"
            aria-label={`${contextMenu.session.hostName} ${contextMenu.session.name} actions`}
            className="fixed z-[10000] min-w-56 rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-lg"
            style={{ left: Math.min(contextMenu.x, window.innerWidth - 240), top: Math.min(contextMenu.y, window.innerHeight - 280) }}
          >
            <button role="menuitem" type="button" onClick={() => startRename(contextMenu.session)} className="flex h-10 w-full items-center rounded px-3 text-left hover:bg-accent focus:bg-accent focus:outline-none">Rename</button>
            <button role="menuitem" type="button" onClick={() => { onTogglePin(contextMenu.session.key); setContextMenu(null) }} className="flex h-10 w-full items-center rounded px-3 text-left hover:bg-accent focus:bg-accent focus:outline-none">{pinnedSet.has(contextMenu.session.key) ? 'Unpin' : 'Pin'}</button>
            <div role="separator" className="my-1 h-px bg-border" />
            <button role="menuitem" type="button" onClick={() => runDetach(contextMenu.session.key)} className="flex h-10 w-full items-center rounded px-3 text-left hover:bg-accent focus:bg-accent focus:outline-none">Detach browser client (tmux keeps running)</button>
            <div role="separator" className="my-1 h-px bg-border" />
            {canKillSession && onKillSession ? (
              <button
                role="menuitem"
                type="button"
                onClick={() => { setKillTarget(contextMenu.session); setContextMenu(null) }}
                className="flex h-10 w-full items-center rounded px-3 text-left text-destructive hover:bg-destructive/10 focus:bg-destructive/10 focus:outline-none"
              >
                End tmux session…
              </button>
            ) : (
              <p className="px-3 py-2 text-[11px] text-muted-foreground">Browser detach only. This Host does not expose a safe kill-session capability.</p>
            )}
          </div>
        )}
      </aside>

      {killTarget && (
        <Dialog
          open
          role="alertdialog"
          onOpenChange={open => { if (!open) setKillTarget(null) }}
          title="End tmux session permanently?"
          description="This terminates the tmux session, not just this browser view."
          footer={(
            <>
              <Button variant="secondary" onClick={() => setKillTarget(null)}>Cancel</Button>
              <Button
                variant="destructive"
                onClick={async () => {
                  const target = killTarget.key
                  setKillTarget(null)
                  await onKillSession?.(target)
                }}
              >
                End tmux session
              </Button>
            </>
          )}
        >
          <dl className="rounded border border-border bg-muted/50 p-3 text-sm">
            <div><dt className="inline text-muted-foreground">Host: </dt><dd className="inline font-mono">{killTarget.hostName} ({killTarget.hostId})</dd></div>
            <div className="mt-1"><dt className="inline text-muted-foreground">Session: </dt><dd className="inline font-mono">{killTarget.name}</dd></div>
          </dl>
        </Dialog>
      )}
    </>
  )
}

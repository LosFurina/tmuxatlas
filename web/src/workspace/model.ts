import type { ActivitySnapshot } from '../hooks/useActivity'
import type { Host } from '../hooks/useHosts'
import type { Session } from '../hooks/useSessions'
import { sessionKey } from '../hooks/useSessions'
import type { ToolEvent } from '../hooks/useToolEvents'

export type WorkspaceStatus = 'running' | 'waiting' | 'done' | 'error' | 'offline'

export interface WorkspaceSession {
  key: string
  hostId: string
  hostName: string
  name: string
  status: WorkspaceStatus
  lastActivity: string | null
  agents: string[]
  activity?: ActivitySnapshot
  searchText: string
  source: Session
}

export interface WorkspaceHost {
  id: string
  name: string
  online: boolean
  status: WorkspaceStatus
  attentionCount: number
  sessionCount: number
  sessions: WorkspaceSession[]
}

export interface WorkspaceViewModel {
  hosts: WorkspaceHost[]
  sessions: WorkspaceSession[]
  byTarget: Map<string, WorkspaceSession>
}

const statusPriority: Record<WorkspaceStatus, number> = {
  error: 5,
  waiting: 4,
  offline: 3,
  running: 2,
  done: 1,
}

const shellCommands = new Set(['bash', 'zsh', 'fish', 'sh', 'dash', 'ksh', 'csh', 'tcsh', 'tmux', 'login'])

function newestTimestamp(values: Array<string | undefined>): string | null {
  const valid = values.filter((value): value is string => Boolean(value) && !Number.isNaN(Date.parse(value!)))
  if (valid.length === 0) return null
  return valid.sort((left, right) => Date.parse(right) - Date.parse(left))[0]
}

function deriveSessionStatus(session: Session, events: ToolEvent[]): WorkspaceStatus {
  if (events.some(event => event.status === 'error')) return 'error'
  if (events.some(event => event.status === 'waiting')) return 'waiting'
  if (session.host_online === false) return 'offline'
  const hasRunningPane = session.windows.some(window => window.panes?.some(pane => (
    Boolean(pane.current_command) && !shellCommands.has(pane.current_command)
  )))
  if (events.some(event => event.status === 'active') || hasRunningPane || session.attached) return 'running'
  return 'done'
}

function highestStatus(sessions: WorkspaceSession[], online: boolean): WorkspaceStatus {
  if (sessions.length === 0) return online ? 'done' : 'offline'
  return sessions.reduce((highest, session) => (
    statusPriority[session.status] > statusPriority[highest] ? session.status : highest
  ), sessions[0].status)
}

export function isAttentionStatus(status: WorkspaceStatus): boolean {
  return status === 'waiting' || status === 'error' || status === 'offline'
}

export function buildWorkspaceViewModel(
  sessions: Session[],
  toolEvents: ToolEvent[],
  activity: Map<string, ActivitySnapshot>,
  hosts: Host[] = [],
): WorkspaceViewModel {
  const hostById = new Map(hosts.map(host => [host.id, host]))
  const viewSessions = sessions.map<WorkspaceSession>(session => {
    const key = sessionKey(session)
    const events = toolEvents.filter(event => event.host === session.host && event.session === session.name)
    const host = hostById.get(session.host)
    const hostName = session.host_name || host?.name || session.host
    const agents = [...new Set(events.map(event => event.tool).filter(Boolean))].sort()
    const lastActivity = newestTimestamp([
      session.last_activity,
      ...events.map(event => event.timestamp),
    ])
    const status = deriveSessionStatus(session, events)
    const activitySnapshot = activity.get(key)
    const searchText = [
      session.host,
      hostName,
      session.name,
      status,
      ...agents,
      ...session.windows.flatMap(window => [window.name, ...window.panes.map(pane => pane.current_command)]),
      activitySnapshot?.idle_seconds === 0 ? 'active' : '',
    ].join(' ').toLowerCase()
    return { key, hostId: session.host, hostName, name: session.name, status, lastActivity, agents, activity: activitySnapshot, searchText, source: session }
  }).sort((left, right) => (
    statusPriority[right.status] - statusPriority[left.status]
    || (Date.parse(right.lastActivity || '') || 0) - (Date.parse(left.lastActivity || '') || 0)
    || left.name.localeCompare(right.name)
    || left.hostId.localeCompare(right.hostId)
  ))

  const hostIds = new Set([...hosts.map(host => host.id), ...viewSessions.map(session => session.hostId)])
  const viewHosts = [...hostIds].map<WorkspaceHost>(id => {
    const host = hostById.get(id)
    const hostSessions = viewSessions.filter(session => session.hostId === id)
    const online = host?.online ?? hostSessions.some(session => session.source.host_online !== false)
    return {
      id,
      name: host?.name || hostSessions[0]?.hostName || id,
      online,
      status: highestStatus(hostSessions, online),
      attentionCount: hostSessions.filter(session => isAttentionStatus(session.status)).length,
      sessionCount: hostSessions.length,
      sessions: hostSessions,
    }
  }).sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id))

  return {
    hosts: viewHosts,
    sessions: viewSessions,
    byTarget: new Map(viewSessions.map(session => [session.key, session])),
  }
}

export function filterWorkspaceSessions(
  sessions: readonly WorkspaceSession[],
  query: string,
  status: WorkspaceStatus | 'all',
): WorkspaceSession[] {
  const normalized = query.trim().toLowerCase()
  return sessions.filter(session => (
    (status === 'all' || session.status === status)
    && (!normalized || session.searchText.includes(normalized))
  ))
}

export function workspaceStatusLabel(status: WorkspaceStatus): string {
  return ({ running: 'Running', waiting: 'Waiting', done: 'Done', error: 'Error', offline: 'Offline' })[status]
}

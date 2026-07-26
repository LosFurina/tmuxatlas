import type { ActivitySnapshot } from '../hooks/useActivity'
import type { Host } from '../hooks/useHosts'
import type { Pane, Session, Window } from '../hooks/useSessions'
import type { ToolEvent } from '../hooks/useToolEvents'
import type { ApplicationState } from './reducer'

export function selectSessions(state: ApplicationState): Session[] {
  return Object.values(state.projection.sessions)
    .sort((left, right) => left.key.localeCompare(right.key))
    .map((source) => {
      const host = state.projection.hosts[source.host_key]
      const windows: Window[] = Object.values(state.projection.windows)
        .filter((window) => window.session_key === source.key)
        .sort((left, right) => left.index - right.index || left.key.localeCompare(right.key))
        .map((window) => {
          const panes: Pane[] = Object.values(state.projection.panes)
            .filter((pane) => pane.window_key === window.key)
            .sort((left, right) => left.index - right.index || left.key.localeCompare(right.key))
            .map((pane) => ({
              id: pane.tmux_id,
              window_id: window.tmux_id,
              session_id: source.tmux_id || '',
              index: pane.index,
              active: pane.active,
              width: pane.width,
              height: pane.height,
              current_command: pane.current_command || '',
              pid: pane.pid || 0,
            }))
          return {
            id: window.tmux_id,
            session_id: source.tmux_id || '',
            name: window.name,
            index: window.index,
            active: window.active,
            layout: window.layout || '',
            panes,
          }
        })
      return {
        id: source.tmux_id || '',
        name: source.name,
        host: source.host_id,
        host_name: host?.display_name,
        host_online: host?.online,
        windows,
        created: source.created || '',
        attached: source.attached,
        last_activity: source.last_activity || '',
      }
    })
}

export function selectHosts(state: ApplicationState, sessions = selectSessions(state)): Host[] {
  return Object.values(state.projection.hosts)
    .sort((left, right) => left.key.localeCompare(right.key))
    .map((host) => ({
      id: host.id,
      name: host.display_name,
      version: host.version,
      local: host.local,
      online: host.online,
      sessions: sessions.filter((session) => session.host === host.id),
      last_seen: host.last_seen || '',
    }))
}

export function selectToolEvents(state: ApplicationState): ToolEvent[] {
  return Object.values(state.projection.tool_events)
    .sort((left, right) => left.key.localeCompare(right.key))
    .map((event) => ({
      tool: event.tool,
      status: event.status as ToolEvent['status'],
      host: event.host_id,
      session: event.session,
      window: Number(event.window || 0),
      pane: event.pane,
      message: event.message,
      timestamp: event.timestamp,
      auto_detected: event.auto_detected,
    }))
}

export function selectActivity(state: ApplicationState): Map<string, ActivitySnapshot> {
  const result = new Map<string, ActivitySnapshot>()
  for (const activity of Object.values(state.projection.activity)) {
    const value: ActivitySnapshot = {
      host: activity.host_id,
      session: activity.session,
      idle_seconds: activity.data?.idle_seconds || 0,
      sparkline: activity.data?.sparkline || [],
      total_bytes: activity.data?.total_bytes || 0,
    }
    result.set(`${activity.host_id}/${activity.session}`, value)
  }
  return result
}

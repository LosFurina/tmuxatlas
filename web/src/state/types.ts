export const STATE_SCHEMA_VERSION = 1

export type ConnectionState =
  | 'connecting'
  | 'rehydrating'
  | 'ready'
  | 'reconnecting'
  | 'auth-required'
  | 'reload-required'

export interface HostState {
  key: string
  id: string
  display_name: string
  online: boolean
  local?: boolean
  version?: string
  last_seen?: string
}

export interface SessionState {
  key: string
  host_key: string
  host_id: string
  name: string
  tmux_id?: string
  attached: boolean
  created?: string
  last_activity?: string
}

export interface WindowState {
  key: string
  session_key: string
  tmux_id: string
  name: string
  index: number
  active: boolean
  layout?: string
}

export interface PaneState {
  key: string
  window_key: string
  tmux_id: string
  index: number
  active: boolean
  width: number
  height: number
  current_command?: string
  pid?: number
}

export interface ToolEventState {
  key: string
  host_id: string
  session: string
  window?: string
  pane?: string
  tool: string
  status: string
  message?: string
  timestamp: string
  auto_detected?: boolean
}

export interface ActivityState {
  key: string
  host_id: string
  session: string
  window?: string
  pane?: string
  timestamp: string
  data?: {
    idle_seconds?: number
    sparkline?: number[]
    total_bytes?: number
    [key: string]: unknown
  }
}

export interface HealthState {
  host_key: string
  last_state_sync?: string
  facts?: Record<string, unknown>
}

export interface Projection {
  hosts: Record<string, HostState>
  sessions: Record<string, SessionState>
  windows: Record<string, WindowState>
  panes: Record<string, PaneState>
  tool_events: Record<string, ToolEventState>
  activity: Record<string, ActivityState>
  health: Record<string, HealthState>
  metadata?: Record<string, unknown>
}

export interface SnapshotEnvelope {
  type: 'snapshot'
  schema_version: number
  instance_id: string
  revision: number
  state: Projection
}

export type StateOperation =
  | { kind: 'upsert-host'; host: HostState }
  | { kind: 'remove-host'; key: string }
  | { kind: 'upsert-session'; session: SessionState }
  | { kind: 'remove-session'; key: string }
  | { kind: 'upsert-window'; window: WindowState }
  | { kind: 'remove-window'; key: string }
  | { kind: 'upsert-pane'; pane: PaneState }
  | { kind: 'remove-pane'; key: string }
  | { kind: 'upsert-tool-event'; tool_event: ToolEventState }
  | { kind: 'remove-tool-event'; key: string }
  | { kind: 'upsert-activity'; activity: ActivityState }
  | { kind: 'remove-activity'; key: string }
  | { kind: 'upsert-health'; health: HealthState }
  | { kind: 'remove-health'; key: string }
  | { kind: 'set-metadata'; key: string; metadata: unknown }
  | { kind: 'remove-metadata'; key: string }

export interface DeltaEnvelope {
  type: 'delta'
  schema_version: number
  instance_id: string
  base_revision: number
  revision: number
  operations: StateOperation[]
}

export interface OutcomeEnvelope {
  type: 'resync-required' | 'reload-required'
  schema_version: number
  instance_id?: string
  revision?: number
  reason?: string
}

export type StateEnvelope = SnapshotEnvelope | DeltaEnvelope | OutcomeEnvelope

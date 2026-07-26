import {
  STATE_SCHEMA_VERSION,
  type ConnectionState,
  type DeltaEnvelope,
  type Projection,
  type SnapshotEnvelope,
  type StateOperation,
} from './types'

export interface ApplicationState {
  schemaVersion: number
  instanceId: string | null
  revision: number
  projection: Projection
  connection: ConnectionState
  rehydrateReason?: string
  reloadReason?: string
}

export type ApplicationStateAction =
  | { type: 'snapshot'; envelope: SnapshotEnvelope }
  | { type: 'delta'; envelope: DeltaEnvelope }
  | { type: 'connection'; state: ConnectionState }
  | { type: 'resync-required'; reason?: string }
  | { type: 'reload-required'; reason?: string }

export function emptyProjection(): Projection {
  return {
    hosts: {},
    sessions: {},
    windows: {},
    panes: {},
    tool_events: {},
    activity: {},
    health: {},
    metadata: {},
  }
}

export const initialApplicationState: ApplicationState = {
  schemaVersion: STATE_SCHEMA_VERSION,
  instanceId: null,
  revision: 0,
  projection: emptyProjection(),
  connection: 'connecting',
}

export function applicationStateReducer(
  state: ApplicationState,
  action: ApplicationStateAction,
): ApplicationState {
  switch (action.type) {
    case 'connection':
      return { ...state, connection: action.state }
    case 'resync-required':
      return {
        ...state,
        connection: 'rehydrating',
        rehydrateReason: action.reason || 'State resynchronization is required.',
      }
    case 'reload-required':
      return {
        ...state,
        connection: 'reload-required',
        reloadReason: action.reason || 'This page must be reloaded to match the Hub.',
      }
    case 'snapshot': {
      const envelope = action.envelope
      if (
        envelope.schema_version !== STATE_SCHEMA_VERSION ||
        !envelope.instance_id ||
        envelope.revision < 0
      ) {
        return {
          ...state,
          connection: 'reload-required',
          reloadReason: 'The Hub state schema is not supported by this page.',
        }
      }
      return {
        schemaVersion: envelope.schema_version,
        instanceId: envelope.instance_id,
        revision: envelope.revision,
        projection: cloneProjection(envelope.state),
        connection: 'ready',
      }
    }
    case 'delta':
      return applyDelta(state, action.envelope)
  }
}

function applyDelta(state: ApplicationState, envelope: DeltaEnvelope): ApplicationState {
  if (envelope.schema_version !== STATE_SCHEMA_VERSION) {
    return {
      ...state,
      connection: 'reload-required',
      reloadReason: 'The Hub state schema changed.',
    }
  }
  if (envelope.revision <= state.revision && envelope.instance_id === state.instanceId) {
    return state
  }
  if (
    !state.instanceId ||
    envelope.instance_id !== state.instanceId ||
    envelope.base_revision !== state.revision ||
    envelope.revision <= envelope.base_revision
  ) {
    return {
      ...state,
      connection: 'rehydrating',
      rehydrateReason:
        envelope.instance_id !== state.instanceId
          ? 'The Hub process restarted.'
          : 'A state revision gap was detected.',
    }
  }

  const projection = cloneProjection(state.projection)
  try {
    for (const operation of envelope.operations) {
      applyOperation(projection, operation)
    }
  } catch {
    return {
      ...state,
      connection: 'rehydrating',
      rehydrateReason: 'A state update could not be applied.',
    }
  }
  return {
    ...state,
    revision: envelope.revision,
    projection,
    connection: 'ready',
    rehydrateReason: undefined,
  }
}

function applyOperation(projection: Projection, operation: StateOperation) {
  switch (operation.kind) {
    case 'upsert-host':
      requireKey(operation.host.key)
      projection.hosts[operation.host.key] = { ...operation.host }
      break
    case 'remove-host':
      delete projection.hosts[requireKey(operation.key)]
      break
    case 'upsert-session':
      requireKey(operation.session.key)
      projection.sessions[operation.session.key] = { ...operation.session }
      break
    case 'remove-session':
      delete projection.sessions[requireKey(operation.key)]
      break
    case 'upsert-window':
      requireKey(operation.window.key)
      projection.windows[operation.window.key] = { ...operation.window }
      break
    case 'remove-window':
      delete projection.windows[requireKey(operation.key)]
      break
    case 'upsert-pane':
      requireKey(operation.pane.key)
      projection.panes[operation.pane.key] = { ...operation.pane }
      break
    case 'remove-pane':
      delete projection.panes[requireKey(operation.key)]
      break
    case 'upsert-tool-event':
      requireKey(operation.tool_event.key)
      projection.tool_events[operation.tool_event.key] = { ...operation.tool_event }
      break
    case 'remove-tool-event':
      delete projection.tool_events[requireKey(operation.key)]
      break
    case 'upsert-activity':
      requireKey(operation.activity.key)
      projection.activity[operation.activity.key] = structuredClone(operation.activity)
      break
    case 'remove-activity':
      delete projection.activity[requireKey(operation.key)]
      break
    case 'upsert-health':
      requireKey(operation.health.host_key)
      projection.health[operation.health.host_key] = structuredClone(operation.health)
      break
    case 'remove-health':
      delete projection.health[requireKey(operation.key)]
      break
    case 'set-metadata':
      projection.metadata ||= {}
      projection.metadata[requireKey(operation.key)] = structuredClone(operation.metadata)
      break
    case 'remove-metadata':
      delete projection.metadata?.[requireKey(operation.key)]
      break
    default:
      throw new Error('Unknown state operation')
  }
}

function requireKey(key: string): string {
  if (!key) throw new Error('Stable state key is required')
  return key
}

function cloneProjection(projection: Projection): Projection {
  return structuredClone({
    ...emptyProjection(),
    ...projection,
    metadata: projection.metadata || {},
  })
}

import {
  createContext,
  type PropsWithChildren,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
} from 'react'
import type { ActivitySnapshot } from '../hooks/useActivity'
import type { Host } from '../hooks/useHosts'
import type { Session } from '../hooks/useSessions'
import type { ToolEvent } from '../hooks/useToolEvents'
import { StateConnectionController } from './connection'
import {
  applicationStateReducer,
  initialApplicationState,
  type ApplicationState,
} from './reducer'
import { selectActivity, selectHosts, selectSessions, selectToolEvents } from './selectors'
import type { StateEnvelope } from './types'

interface ApplicationStateContextValue {
  state: ApplicationState
  sessions: Session[]
  hosts: Host[]
  toolEvents: ToolEvent[]
  activity: Map<string, ActivitySnapshot>
  rehydrate: () => void
}

const ApplicationStateContext = createContext<ApplicationStateContextValue | null>(null)

export function ApplicationStateProvider({ children }: PropsWithChildren) {
  const [state, dispatch] = useReducer(applicationStateReducer, initialApplicationState)
  const stateRef = useRef(state)
  const controllerRef = useRef<StateConnectionController | null>(null)
  stateRef.current = state

  useEffect(() => {
    const controller = new StateConnectionController({
      onStatus: (connection) => dispatch({ type: 'connection', state: connection }),
      onEnvelope: (envelope: StateEnvelope) => {
        if (envelope.type === 'snapshot') {
          dispatch({ type: 'snapshot', envelope })
          return
        }
        if (envelope.type === 'delta') {
          const current = stateRef.current
          const gap =
            current.instanceId !== envelope.instance_id ||
            (envelope.revision > current.revision &&
              envelope.base_revision !== current.revision)
          dispatch({ type: 'delta', envelope })
          if (gap) queueMicrotask(() => controller.rehydrate('State revision gap detected.'))
          return
        }
        if (envelope.type === 'resync-required') {
          dispatch({ type: 'resync-required', reason: envelope.reason })
          return
        }
        dispatch({ type: 'reload-required', reason: envelope.reason })
      },
    })
    controllerRef.current = controller
    controller.start()
    return () => {
      controllerRef.current = null
      controller.dispose()
    }
  }, [])

  const sessions = useMemo(() => selectSessions(state), [state])
  const hosts = useMemo(() => selectHosts(state, sessions), [state, sessions])
  const toolEvents = useMemo(() => selectToolEvents(state), [state])
  const activity = useMemo(() => selectActivity(state), [state])
  const rehydrate = useCallback(() => controllerRef.current?.rehydrate(), [])
  const value = useMemo(() => ({
    state, sessions, hosts, toolEvents, activity, rehydrate,
  }), [state, sessions, hosts, toolEvents, activity, rehydrate])

  return (
    <ApplicationStateContext.Provider value={value}>
      {children}
    </ApplicationStateContext.Provider>
  )
}

export function useApplicationState() {
  const context = useContext(ApplicationStateContext)
  if (!context) throw new Error('useApplicationState must be used within ApplicationStateProvider')
  return context
}

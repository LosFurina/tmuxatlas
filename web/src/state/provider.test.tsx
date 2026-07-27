import { act, renderHook } from '@testing-library/react'
import { afterEach, vi, describe, expect, it } from 'vitest'
import { useApplicationState, ApplicationStateProvider } from './provider'
import { renderHookStrict } from '../test/render'

class FakeSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static readonly instances: FakeSocket[] = []

  readyState = FakeSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  constructor(_url: string | URL) {
    FakeSocket.instances.push(this)
  }

  open() {
    this.readyState = FakeSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  message(value: unknown) {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(value) }))
  }

  close() {
    if (this.readyState === FakeSocket.CLOSED) return
    this.readyState = FakeSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }

  send() {}
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  FakeSocket.instances.length = 0
})

describe('ApplicationStateProvider', () => {
  it('does not issue an authoritative HTTP bootstrap that can overwrite revisions', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const { unmount } = renderHookStrict(() => useApplicationState(), {
      wrapper: ({ children }) => (
        <ApplicationStateProvider>{children}</ApplicationStateProvider>
      ),
    })
    expect(fetchSpy).not.toHaveBeenCalled()
    unmount()
  })

  it('advances its protocol revision synchronously across batched deltas', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('WebSocket', FakeSocket)

    const { result, unmount } = renderHook(() => useApplicationState(), {
      wrapper: ({ children }) => (
        <ApplicationStateProvider>{children}</ApplicationStateProvider>
      ),
    })
    const socket = FakeSocket.instances[0]

    act(() => {
      socket.open()
      socket.message({
        type: 'snapshot',
        schema_version: 1,
        instance_id: 'hub-a',
        revision: 10,
        state: {
          hosts: {},
          sessions: {},
          windows: {},
          panes: {},
          tool_events: {},
          activity: {},
          health: {},
        },
      })
    })

    act(() => {
      socket.message({
        type: 'delta',
        schema_version: 1,
        instance_id: 'hub-a',
        base_revision: 10,
        revision: 11,
        operations: [],
      })
      socket.message({
        type: 'delta',
        schema_version: 1,
        instance_id: 'hub-a',
        base_revision: 11,
        revision: 12,
        operations: [],
      })
    })
    await act(async () => {
      await Promise.resolve()
      vi.runOnlyPendingTimers()
    })

    expect(result.current.state.revision).toBe(12)
    expect(result.current.state.connection).toBe('ready')
    expect(FakeSocket.instances).toHaveLength(1)
    expect(socket.readyState).toBe(FakeSocket.OPEN)
    unmount()
  })
})

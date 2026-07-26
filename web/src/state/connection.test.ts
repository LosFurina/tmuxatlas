import { afterEach, describe, expect, it, vi } from 'vitest'
import { StateConnectionController } from './connection'
import type { StateEnvelope } from './types'

class FakeSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  readyState = FakeSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  closed = false

  open() {
    this.readyState = FakeSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  message(value: unknown) {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(value) }))
  }

  close() {
    if (this.closed) return
    this.closed = true
    this.readyState = FakeSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }

  send() {}
}

afterEach(() => {
  vi.useRealTimers()
  Object.defineProperty(document, 'hidden', { configurable: true, value: false })
})

function harness() {
  const sockets: FakeSocket[] = []
  const statuses: string[] = []
  const envelopes: StateEnvelope[] = []
  const controller = new StateConnectionController({
    createSocket: () => {
      const socket = new FakeSocket()
      sockets.push(socket)
      return socket as unknown as WebSocket
    },
    random: () => 0.5,
    onStatus: (status) => statuses.push(status),
    onEnvelope: (envelope) => envelopes.push(envelope),
  })
  return { controller, sockets, statuses, envelopes }
}

describe('StateConnectionController', () => {
  it('does not report ready until a snapshot is applied', () => {
    const { controller, sockets, statuses } = harness()
    controller.start()
    sockets[0].open()
    expect(statuses[statuses.length - 1]).toBe('rehydrating')
    sockets[0].message({
      type: 'snapshot', schema_version: 1, instance_id: 'a', revision: 0,
      state: { hosts: {}, sessions: {}, windows: {}, panes: {}, tool_events: {}, activity: {}, health: {} },
    })
    expect(statuses[statuses.length - 1]).toBe('ready')
    controller.dispose()
  })

  it('keeps one socket across pageshow and visibility events while connecting', () => {
    const { controller, sockets } = harness()
    controller.start()
    window.dispatchEvent(new PageTransitionEvent('pageshow'))
    document.dispatchEvent(new Event('visibilitychange'))
    expect(sockets).toHaveLength(1)
    controller.dispose()
  })

  it('invalidates old close handlers and timers on dispose', () => {
    vi.useFakeTimers()
    const { controller, sockets } = harness()
    controller.start()
    const staleClose = sockets[0].onclose
    controller.dispose()
    staleClose?.(new CloseEvent('close'))
    vi.runAllTimers()
    expect(sockets).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('uses one capped reconnect timer and rehydrates a new generation', () => {
    vi.useFakeTimers()
    const { controller, sockets } = harness()
    controller.start()
    sockets[0].open()
    sockets[0].message({
      type: 'snapshot', schema_version: 1, instance_id: 'a', revision: 0,
      state: { hosts: {}, sessions: {}, windows: {}, panes: {}, tool_events: {}, activity: {}, health: {} },
    })
    sockets[0].close()
    document.dispatchEvent(new Event('visibilitychange'))
    window.dispatchEvent(new PageTransitionEvent('pageshow'))
    expect(vi.getTimerCount()).toBe(1)
    vi.advanceTimersByTime(500)
    expect(sockets).toHaveLength(2)
    controller.dispose()
  })
})

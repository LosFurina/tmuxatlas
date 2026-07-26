import { STATE_SCHEMA_VERSION, type StateEnvelope } from './types'

type ConnectionStatus =
  | 'connecting'
  | 'rehydrating'
  | 'ready'
  | 'reconnecting'
  | 'auth-required'
  | 'reload-required'

interface ControllerOptions {
  path?: string
  createSocket?: (url: string) => WebSocket
  setTimer?: typeof window.setTimeout
  clearTimer?: typeof window.clearTimeout
  random?: () => number
  onEnvelope: (envelope: StateEnvelope) => void
  onStatus: (status: ConnectionStatus) => void
}

export class StateConnectionController {
  private readonly path: string
  private readonly createSocket: (url: string) => WebSocket
  private readonly setTimer: typeof window.setTimeout
  private readonly clearTimer: typeof window.clearTimeout
  private readonly random: () => number
  private readonly onEnvelope: (envelope: StateEnvelope) => void
  private readonly onStatus: (status: ConnectionStatus) => void
  private socket: WebSocket | null = null
  private reconnectTimer: number | null = null
  private generation = 0
  private attempts = 0
  private disposed = false
  private hasReadySnapshot = false

  constructor(options: ControllerOptions) {
    this.path = options.path || '/ws/events'
    this.createSocket = options.createSocket || ((url) => new WebSocket(url))
    this.setTimer = options.setTimer || window.setTimeout.bind(window)
    this.clearTimer = options.clearTimer || window.clearTimeout.bind(window)
    this.random = options.random || Math.random
    this.onEnvelope = options.onEnvelope
    this.onStatus = options.onStatus
  }

  start() {
    if (this.disposed) return
    document.addEventListener('visibilitychange', this.onPageAvailable)
    window.addEventListener('pageshow', this.onPageAvailable)
    this.connect()
  }

  rehydrate(reason = 'State synchronization was lost.') {
    if (this.disposed) return
    this.onStatus('rehydrating')
    this.restart(0, reason)
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    this.generation++
    document.removeEventListener('visibilitychange', this.onPageAvailable)
    window.removeEventListener('pageshow', this.onPageAvailable)
    this.cancelTimer()
    const socket = this.socket
    this.socket = null
    if (socket) {
      socket.onopen = null
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      socket.close()
    }
  }

  private readonly onPageAvailable = () => {
    if (this.disposed || document.hidden) return
    if (
      this.reconnectTimer === null &&
      (!this.socket ||
        (this.socket.readyState !== WebSocket.OPEN &&
          this.socket.readyState !== WebSocket.CONNECTING))
    ) {
      this.connect()
    }
  }

  private connect() {
    if (this.disposed || document.hidden || this.reconnectTimer !== null) return
    if (
      this.socket &&
      (this.socket.readyState === WebSocket.OPEN ||
        this.socket.readyState === WebSocket.CONNECTING)
    ) {
      return
    }

    const generation = ++this.generation
    this.onStatus(this.hasReadySnapshot ? 'reconnecting' : 'connecting')
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const query = `schema=${STATE_SCHEMA_VERSION}`
    const socket = this.createSocket(
      `${protocol}//${window.location.host}${this.path}?${query}`,
    )
    this.socket = socket

    socket.onopen = () => {
      if (!this.isCurrent(generation, socket)) return
      this.onStatus('rehydrating')
    }
    socket.onmessage = (event) => {
      if (!this.isCurrent(generation, socket)) return
      let envelope: StateEnvelope
      try {
        envelope = JSON.parse(String(event.data)) as StateEnvelope
      } catch {
        this.rehydrate('The Hub sent malformed state data.')
        return
      }
      if (envelope.type === 'reload-required') {
        this.onEnvelope(envelope)
        this.onStatus('reload-required')
        this.stopSocket(generation, socket)
        return
      }
      if (envelope.type === 'resync-required') {
        this.onEnvelope(envelope)
        this.rehydrate(envelope.reason)
        return
      }
      this.onEnvelope(envelope)
      if (envelope.type === 'snapshot') {
        this.hasReadySnapshot = true
        this.attempts = 0
        this.onStatus('ready')
      }
    }
    socket.onerror = () => {
      if (this.isCurrent(generation, socket)) socket.close()
    }
    socket.onclose = (event) => {
      if (!this.isCurrent(generation, socket)) return
      this.socket = null
      if (event.code === 4401 || event.code === 4403) {
        this.onStatus('auth-required')
        return
      }
      if (!document.hidden) this.scheduleReconnect()
    }
  }

  private restart(delay: number, _reason: string) {
    this.cancelTimer()
    const socket = this.socket
    const oldGeneration = this.generation
    this.socket = null
    this.generation++
    if (socket) {
      socket.onopen = null
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      socket.close()
    }
    if (this.disposed || document.hidden) return
    this.reconnectTimer = this.setTimer(() => {
      this.reconnectTimer = null
      if (!this.disposed && this.generation === oldGeneration + 1) this.connect()
    }, delay)
  }

  private scheduleReconnect() {
    if (this.disposed || this.reconnectTimer !== null) return
    this.onStatus(this.hasReadySnapshot ? 'reconnecting' : 'connecting')
    const capped = Math.min(30_000, 500 * 2 ** Math.min(this.attempts, 6))
    this.attempts++
    const jittered = Math.round(capped * (0.8 + this.random() * 0.4))
    const generation = this.generation
    this.reconnectTimer = this.setTimer(() => {
      this.reconnectTimer = null
      if (!this.disposed && this.generation === generation) this.connect()
    }, jittered)
  }

  private cancelTimer() {
    if (this.reconnectTimer !== null) {
      this.clearTimer(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }

  private stopSocket(generation: number, socket: WebSocket) {
    if (!this.isCurrent(generation, socket)) return
    this.generation++
    this.socket = null
    socket.onopen = null
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
  }

  private isCurrent(generation: number, socket: WebSocket) {
    return !this.disposed && this.generation === generation && this.socket === socket
  }
}

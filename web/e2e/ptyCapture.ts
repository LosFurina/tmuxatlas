import type { Page, WebSocketRoute } from '@playwright/test'

export interface PTYTarget {
  hostId: string
  sessionName: string
}

export interface CapturedPTYConnection {
  generation: number
  url: string
  hostId: string
  sessionName: string
  messages: (string | Buffer)[]
  socket: WebSocketRoute
}

export interface CapturedPTYBinaryFrame extends PTYTarget {
  /** Monotonically increasing across every mocked PTY connection on the page. */
  ordinal: number
  generation: number
  bytes: Buffer
  connection: CapturedPTYConnection
}

export interface PTYConnectionWaitOptions {
  /** Ignore an existing connection and wait for a strictly newer generation. */
  afterGeneration?: number
  timeoutMs?: number
}

export interface PTYFrameWaitOptions {
  target: PTYTarget
  generation: number
  /** Ignore frames captured at or before this global frame ordinal. */
  afterOrdinal?: number
  count?: number
  timeoutMs?: number
}

export interface PTYCapture {
  connections: CapturedPTYConnection[]
  connectionsFor(target: PTYTarget): CapturedPTYConnection[]
  latest(target?: PTYTarget): CapturedPTYConnection | undefined
  waitForConnection(
    target: PTYTarget,
    options?: PTYConnectionWaitOptions,
  ): Promise<CapturedPTYConnection>
  binaryFrames(connection: CapturedPTYConnection): Buffer[]
  textFrames(connection: CapturedPTYConnection): string[]
  inputFrames(target?: PTYTarget, generation?: number): CapturedPTYBinaryFrame[]
  /** Returns the latest global frame ordinal for race-free before/after assertions. */
  mark(): number
  waitForInputFrames(options: PTYFrameWaitOptions): Promise<CapturedPTYBinaryFrame[]>
}

function isTarget(connection: PTYTarget, target: PTYTarget): boolean {
  return connection.hostId === target.hostId && connection.sessionName === target.sessionName
}

async function waitUntil<T>(
  read: () => T | undefined,
  description: string,
  timeoutMs = 5_000,
): Promise<T> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() <= deadline) {
    const result = read()
    if (result !== undefined) return result
    await new Promise(resolve => setTimeout(resolve, 10))
  }
  throw new Error(`Timed out waiting for ${description} after ${timeoutMs}ms`)
}

export async function capturePTY(page: Page): Promise<PTYCapture> {
  const connections: CapturedPTYConnection[] = []
  const capturedInputFrames: CapturedPTYBinaryFrame[] = []
  let nextGeneration = 0
  let nextFrameOrdinal = 0

  await page.routeWebSocket('**/ws/session**', socket => {
    const url = new URL(socket.url())
    const hostId = url.searchParams.get('host')
    const sessionName = url.searchParams.get('name')
    if (!hostId || !sessionName) {
      throw new Error(`PTY WebSocket is missing its stable target: ${socket.url()}`)
    }
    const connection: CapturedPTYConnection = {
      generation: ++nextGeneration,
      url: socket.url(),
      hostId,
      sessionName,
      messages: [],
      socket,
    }
    connections.push(connection)
    socket.onMessage(message => {
      const captured = Buffer.isBuffer(message) ? Buffer.from(message) : message
      connection.messages.push(captured)
      if (Buffer.isBuffer(captured)) {
        capturedInputFrames.push({
          ordinal: ++nextFrameOrdinal,
          generation: connection.generation,
          hostId: connection.hostId,
          sessionName: connection.sessionName,
          bytes: captured,
          connection,
        })
      }
    })
  })

  return {
    connections,
    connectionsFor(target) {
      return connections.filter(connection => isTarget(connection, target))
    },
    latest(target) {
      return connections.findLast(connection => (
        !target
        || isTarget(connection, target)
      ))
    },
    waitForConnection(target, options = {}) {
      return waitUntil(
        () => connections.findLast(connection => (
          isTarget(connection, target)
          && connection.generation > (options.afterGeneration ?? 0)
        )),
        `PTY connection ${target.hostId}/${target.sessionName}`,
        options.timeoutMs,
      )
    },
    binaryFrames(connection) {
      return connection.messages.filter((message): message is Buffer => Buffer.isBuffer(message))
    },
    textFrames(connection) {
      return connection.messages.filter((message): message is string => typeof message === 'string')
    },
    inputFrames(target, generation) {
      return capturedInputFrames.filter(frame => (
        (!target || isTarget(frame, target))
        && (generation === undefined || frame.generation === generation)
      ))
    },
    mark() {
      return nextFrameOrdinal
    },
    waitForInputFrames({
      target,
      generation,
      afterOrdinal = 0,
      count = 1,
      timeoutMs,
    }) {
      return waitUntil(
        () => {
          const matches = capturedInputFrames.filter(frame => (
            isTarget(frame, target)
            && frame.generation === generation
            && frame.ordinal > afterOrdinal
          ))
          return matches.length >= count ? matches : undefined
        },
        `${count} Binary PTY frame(s) for ${target.hostId}/${target.sessionName} generation ${generation}`,
        timeoutMs,
      )
    },
  }
}

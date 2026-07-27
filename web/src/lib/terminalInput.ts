export const MAX_TERMINAL_COMMAND_BODY_BYTES = 65_535

export type TerminalInputErrorCode =
  | 'body-too-large'
  | 'not-connected'
  | 'stale-connection'
  | 'send-failed'

export class TerminalInputError extends Error {
  readonly code: TerminalInputErrorCode

  constructor(code: TerminalInputErrorCode, message: string) {
    super(message)
    this.name = 'TerminalInputError'
    this.code = code
  }
}

export interface TerminalConnectionCapture {
  targetKey: string
  generation: number
}

const encoder = new TextEncoder()

export function terminalTargetKey(hostId: string, sessionName: string): string {
  return JSON.stringify([hostId, sessionName])
}

export function terminalCommandBodyByteLength(value: string): number {
  return encoder.encode(value).byteLength
}

/**
 * Encode a Mobile Composer submission without interpreting or normalizing it.
 * The body is preserved byte-for-byte as UTF-8 and exactly one CR is appended.
 */
export function encodeTerminalCommand(value: string): Uint8Array {
  const body = encoder.encode(value)
  if (body.byteLength > MAX_TERMINAL_COMMAND_BODY_BYTES) {
    throw new TerminalInputError(
      'body-too-large',
      `Command is ${body.byteLength.toLocaleString()} UTF-8 bytes; the maximum is ${MAX_TERMINAL_COMMAND_BODY_BYTES.toLocaleString()}.`,
    )
  }

  const frame = new Uint8Array(body.byteLength + 1)
  frame.set(body)
  frame[frame.byteLength - 1] = 0x0d
  return frame
}

export function isMultilineTerminalPaste(value: string): boolean {
  return value.includes('\n') || value.includes('\r')
}

export function encodeTerminalPaste(value: string, bracketed: boolean): Uint8Array {
  return encoder.encode(bracketed ? `\x1b[200~${value}\x1b[201~` : value)
}

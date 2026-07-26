export type ModifierMode = 'off' | 'once' | 'locked'
export type ModifierName = 'ctrl' | 'alt'

export interface ModifierState {
  ctrl: ModifierMode
  alt: ModifierMode
}

const nextMode: Record<ModifierMode, ModifierMode> = {
  off: 'once',
  once: 'locked',
  locked: 'off',
}

export class MobileTerminalInput {
  private state: ModifierState = { ctrl: 'off', alt: 'off' }
  private listeners = new Set<(state: ModifierState) => void>()

  snapshot(): ModifierState {
    return { ...this.state }
  }

  subscribe(listener: (state: ModifierState) => void): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  cycle(modifier: ModifierName): void {
    this.state = { ...this.state, [modifier]: nextMode[this.state[modifier]] }
    this.emit()
  }

  reset(): void {
    this.state = { ctrl: 'off', alt: 'off' }
    this.emit()
  }

  encode(data: string): Uint8Array {
    let value = data
    if (this.state.ctrl !== 'off' && value.length === 1) {
      const code = value.toUpperCase().charCodeAt(0)
      if (code >= 64 && code <= 95) value = String.fromCharCode(code & 0x1f)
    }
    if (this.state.alt !== 'off') value = `\x1b${value}`
    const consumed = this.state.ctrl === 'once' || this.state.alt === 'once'
    if (consumed) {
      this.state = {
        ctrl: this.state.ctrl === 'once' ? 'off' : this.state.ctrl,
        alt: this.state.alt === 'once' ? 'off' : this.state.alt,
      }
      this.emit()
    }
    return new TextEncoder().encode(value)
  }

  private emit(): void {
    const snapshot = this.snapshot()
    this.listeners.forEach(listener => listener(snapshot))
  }
}

export const terminalKeys = {
  escape: '\x1b',
  tab: '\t',
  up: '\x1b[A',
  down: '\x1b[B',
  right: '\x1b[C',
  left: '\x1b[D',
} as const

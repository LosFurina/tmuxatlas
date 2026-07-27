import { describe, expect, it, vi } from 'vitest'
import { MobileTerminalInput, terminalKeys } from './mobileTerminalInput'

const decode = (value: Uint8Array) => new TextDecoder().decode(value)

describe('MobileTerminalInput', () => {
  it('encodes special keys and one-shot Ctrl/Alt', () => {
    const input = new MobileTerminalInput()
    expect(decode(input.encode(terminalKeys.up))).toBe('\x1b[A')
    input.cycle('ctrl')
    expect(decode(input.encode('c'))).toBe('\x03')
    expect(input.snapshot().ctrl).toBe('off')
    input.cycle('alt')
    expect(decode(input.encode('x'))).toBe('\x1bx')
    expect(input.snapshot().alt).toBe('off')
  })

  it('supports locked modifiers and resets on target change', () => {
    const input = new MobileTerminalInput()
    input.cycle('ctrl')
    input.cycle('ctrl')
    expect(decode(input.encode('d'))).toBe('\x04')
    expect(input.snapshot().ctrl).toBe('locked')
    const listener = vi.fn()
    input.subscribe(listener)
    input.reset()
    expect(input.snapshot()).toEqual({ ctrl: 'off', alt: 'off' })
    expect(listener).toHaveBeenCalled()
  })

  it('clears one-shot modifiers without changing locked modifiers', () => {
    const input = new MobileTerminalInput()
    input.cycle('ctrl')
    input.cycle('alt')
    input.cycle('alt')
    input.consumeOneShot()
    expect(input.snapshot()).toEqual({ ctrl: 'off', alt: 'locked' })
  })
})

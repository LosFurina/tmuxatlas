import { describe, expect, it } from 'vitest'
import {
  MAX_TERMINAL_COMMAND_BODY_BYTES,
  TerminalInputError,
  encodeTerminalCommand,
  encodeTerminalPaste,
  terminalCommandBodyByteLength,
  terminalTargetKey,
} from './terminalInput'

const decode = (value: Uint8Array) => new TextDecoder().decode(value)

describe('terminal input framing', () => {
  it.each([
    ['leading and trailing spaces', '  printf ok  '],
    ['quotes and shell metacharacters', `'a' "$HOME" && echo $(date); * ?`],
    ['multiple lines', 'first\nsecond\r\nthird'],
    ['Chinese and emoji', '你好 👩🏽‍💻 🚀'],
    ['empty input', ''],
  ])('preserves %s and appends exactly one CR', (_name, value) => {
    const frame = encodeTerminalCommand(value)
    expect(decode(frame)).toBe(`${value}\r`)
    expect(frame[frame.byteLength - 1]).toBe(0x0d)
    if (value.endsWith('\r')) {
      expect(frame.slice(-2)).toEqual(new Uint8Array([0x0d, 0x0d]))
    }
  })

  it('accepts a 65,535-byte body as one 65,536-byte frame', () => {
    const value = 'x'.repeat(MAX_TERMINAL_COMMAND_BODY_BYTES)
    const frame = encodeTerminalCommand(value)
    expect(frame).toHaveLength(65_536)
    expect(frame[65_535]).toBe(0x0d)
  })

  it('rejects a 65,536-byte body by UTF-8 byte length', () => {
    expect(() => encodeTerminalCommand('x'.repeat(65_536))).toThrowError(TerminalInputError)
    expect(() => encodeTerminalCommand('你'.repeat(21_846))).toThrow(/maximum/i)
  })

  it('counts UTF-8 bytes instead of UTF-16 code units', () => {
    expect(terminalCommandBodyByteLength('你')).toBe(3)
    expect(terminalCommandBodyByteLength('😀')).toBe(4)
  })

  it('only wraps clipboard paste when bracketed paste mode is active', () => {
    expect(decode(encodeTerminalPaste('a\nb', false))).toBe('a\nb')
    expect(decode(encodeTerminalPaste('a\nb', true))).toBe('\x1b[200~a\nb\x1b[201~')
  })

  it('uses the complete stable target for draft isolation', () => {
    expect(terminalTargetKey('host-a', 'work')).not.toBe(terminalTargetKey('host-b', 'work'))
    expect(terminalTargetKey('a/b', 'c')).not.toBe(terminalTargetKey('a', 'b/c'))
  })
})

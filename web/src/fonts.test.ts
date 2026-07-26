import { describe, expect, it, vi } from 'vitest'
import { bundledFontNames, ensureTerminalFont } from './fonts'

describe('self-hosted fonts', () => {
  it('maps every bundled terminal option to a local loader', async () => {
    expect(bundledFontNames).toEqual(['Space Mono', 'JetBrains Mono', 'Fira Code'])
    expect(await ensureTerminalFont('Menlo')).toBe(false)
  })

  it('contains no runtime Google Fonts request', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    await ensureTerminalFont('JetBrains Mono')
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})

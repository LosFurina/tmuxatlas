import { describe, expect, it } from 'vitest'
import { formatLoadValue, formatMemoryMb } from './overviewFormat'

describe('overview format helpers', () => {
  it('falls back to zero for missing load and memory values', () => {
    expect(formatLoadValue(undefined)).toBe('0.00')
    expect(formatMemoryMb(undefined)).toBe('0.0')
  })
})

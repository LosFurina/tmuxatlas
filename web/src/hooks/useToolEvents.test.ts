import { describe, expect, it } from 'vitest'
import { applyToolEvent, type ToolEvent } from './useToolEvents'

const opencodeWaiting: ToolEvent = {
  tool: 'opencode', status: 'waiting', session: 'AGY', window: 0, pane: '%1', timestamp: '2026-09-01T00:00:00Z',
}

describe('applyToolEvent', () => {
  it('keeps a replacement tool waiting when passive Agy completes on the same pane', () => {
    const result = applyToolEvent([opencodeWaiting], {
      tool: 'agy', status: 'completed', session: 'AGY', window: 0, pane: '%1', timestamp: '2026-09-01T00:00:01Z', auto_detected: true,
    })
    expect(result).toEqual([opencodeWaiting])
  })
})

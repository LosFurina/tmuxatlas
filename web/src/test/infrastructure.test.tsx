import { useEffect, useState } from 'react'
import { act } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { renderHookStrict } from './render'

describe('frontend test infrastructure', () => {
  it('renders hooks in StrictMode and supports fake timers', () => {
    vi.useFakeTimers()
    const { result, unmount } = renderHookStrict(() => {
      const [ready, setReady] = useState(false)
      useEffect(() => {
        const timer = window.setTimeout(() => setReady(true), 10)
        return () => window.clearTimeout(timer)
      }, [])
      return ready
    })
    act(() => vi.advanceTimersByTime(10))
    expect(result.current).toBe(true)
    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })
})

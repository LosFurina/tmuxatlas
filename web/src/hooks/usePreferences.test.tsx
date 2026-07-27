import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defaultPreferences, usePreferencesProvider, type Preferences } from './usePreferences'

function response(preferences: Preferences, status = 200) {
  return new Response(JSON.stringify(preferences), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('usePreferencesProvider', () => {
  it('rolls back a failed optimistic save and can retry the same payload', async () => {
    const confirmed = { ...defaultPreferences, theme: 'dark' }
    const desired = { ...confirmed, theme: 'light' }
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response(confirmed))
      .mockResolvedValueOnce(new Response(null, { status: 503 }))
      .mockResolvedValueOnce(response(desired))
    vi.stubGlobal('fetch', fetcher)

    const { result } = renderHook(() => usePreferencesProvider())
    await waitFor(() => expect(result.current.loaded).toBe(true))

    await act(async () => {
      await result.current.updatePrefs({ theme: 'light' })
    })
    expect(result.current.prefs.theme).toBe('dark')
    expect(result.current.saveState).toBe('error')
    expect(result.current.saveError).toMatch(/503/)

    await act(async () => {
      await result.current.retrySave()
    })
    expect(result.current.prefs.theme).toBe('light')
    expect(result.current.saveState).toBe('saved')
    expect(result.current.saveError).toBeNull()
  })

  it('serializes rapid saves and keeps the latest confirmed value', async () => {
    const first = { ...defaultPreferences, theme: 'dark' }
    const second = { ...defaultPreferences, theme: 'light' }
    let resolveFirst!: (value: Response) => void
    const firstPut = new Promise<Response>((resolve) => { resolveFirst = resolve })
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response(defaultPreferences))
      .mockReturnValueOnce(firstPut)
      .mockResolvedValueOnce(response(second))
    vi.stubGlobal('fetch', fetcher)

    const { result } = renderHook(() => usePreferencesProvider())
    await waitFor(() => expect(result.current.loaded).toBe(true))

    let firstSave!: Promise<void>
    let secondSave!: Promise<void>
    act(() => {
      firstSave = result.current.updatePrefs({ theme: 'dark' })
      secondSave = result.current.updatePrefs({ theme: 'light' })
    })
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2))
    expect(result.current.prefs.theme).toBe('light')

    resolveFirst(response(first))
    await act(async () => {
      await firstSave
      await secondSave
    })

    expect(fetcher).toHaveBeenCalledTimes(3)
    expect(result.current.prefs.theme).toBe('light')
    expect(result.current.saveState).toBe('saved')
  })
})

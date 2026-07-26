import { vi, describe, expect, it } from 'vitest'
import { useApplicationState, ApplicationStateProvider } from './provider'
import { renderHookStrict } from '../test/render'

describe('ApplicationStateProvider', () => {
  it('does not issue an authoritative HTTP bootstrap that can overwrite revisions', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const { unmount } = renderHookStrict(() => useApplicationState(), {
      wrapper: ({ children }) => (
        <ApplicationStateProvider>{children}</ApplicationStateProvider>
      ),
    })
    expect(fetchSpy).not.toHaveBeenCalled()
    unmount()
  })
})

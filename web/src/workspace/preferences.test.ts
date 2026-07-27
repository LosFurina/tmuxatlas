import { beforeEach, describe, expect, it } from 'vitest'
import { loadWorkspacePreferences, saveWorkspacePreferences } from './preferences'

describe('Workspace local preferences', () => {
  beforeEach(() => {
    const values = new Map<string, string>()
    const storage: Storage = {
      get length() { return values.size },
      clear: () => values.clear(),
      getItem: key => values.get(key) ?? null,
      key: index => [...values.keys()][index] ?? null,
      removeItem: key => { values.delete(key) },
      setItem: (key, value) => { values.set(key, String(value)) },
    }
    Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: storage })
  })

  it('persists only canonical pin/recent targets in a namespaced payload', () => {
    saveWorkspacePreferences({
      pinned: ['host-a/work', 'invalid', 'host-a/work'],
      recent: ['host-b/work', '', 'host-a/other'],
    })
    const raw = localStorage.getItem('tmuxatlas:workspace-preferences:v1')
    expect(raw).not.toBeNull()
    expect(JSON.parse(raw || '{}')).toEqual({ pinned: ['host-a/work'], recent: ['host-b/work', 'host-a/other'] })
    expect(raw).not.toContain('draft')
    expect(raw).not.toContain('input')
  })

  it('sanitizes untrusted storage and ignores Terminal or Composer fields', () => {
    localStorage.setItem('tmuxatlas:workspace-preferences:v1', JSON.stringify({
      pinned: ['host-a/work', 2],
      recent: ['host-b/work'],
      terminalInput: 'secret',
      composerDraft: 'token',
    }))
    expect(loadWorkspacePreferences()).toEqual({ pinned: ['host-a/work'], recent: ['host-b/work'] })
  })
})

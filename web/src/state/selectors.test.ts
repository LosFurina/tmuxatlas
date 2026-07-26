import { describe, expect, it } from 'vitest'
import { initialApplicationState } from './reducer'
import { selectSessions } from './selectors'

describe('state selectors', () => {
  it('keeps same-named sessions on separate hosts distinct', () => {
    const state = structuredClone(initialApplicationState)
    state.projection.hosts = {
      'host/a': { key: 'host/a', id: 'a', display_name: 'same', online: true },
      'host/b': { key: 'host/b', id: 'b', display_name: 'same', online: true },
    }
    state.projection.sessions = {
      'host/a/session/work': {
        key: 'host/a/session/work', host_key: 'host/a', host_id: 'a',
        name: 'work', attached: false,
      },
      'host/b/session/work': {
        key: 'host/b/session/work', host_key: 'host/b', host_id: 'b',
        name: 'work', attached: false,
      },
    }
    expect(selectSessions(state).map((session) => `${session.host}/${session.name}`))
      .toEqual(['a/work', 'b/work'])
  })
})

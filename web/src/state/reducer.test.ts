import { describe, expect, it } from 'vitest'
import {
  applicationStateReducer,
  emptyProjection,
  initialApplicationState,
} from './reducer'
import type { DeltaEnvelope, SnapshotEnvelope } from './types'

const snapshot = (instance = 'instance-a', revision = 4): SnapshotEnvelope => ({
  type: 'snapshot',
  schema_version: 1,
  instance_id: instance,
  revision,
  state: emptyProjection(),
})

describe('applicationStateReducer', () => {
  it('replaces projection from a valid snapshot and becomes ready', () => {
    const state = applicationStateReducer(initialApplicationState, {
      type: 'snapshot',
      envelope: snapshot(),
    })
    expect(state.instanceId).toBe('instance-a')
    expect(state.revision).toBe(4)
    expect(state.connection).toBe('ready')
  })

  it('applies ordered deltas and ignores duplicate revisions', () => {
    const ready = applicationStateReducer(initialApplicationState, {
      type: 'snapshot',
      envelope: snapshot(),
    })
    const delta: DeltaEnvelope = {
      type: 'delta',
      schema_version: 1,
      instance_id: 'instance-a',
      base_revision: 4,
      revision: 5,
      operations: [{
        kind: 'upsert-host',
        host: { key: 'host/a', id: 'a', display_name: 'same', online: true },
      }],
    }
    const updated = applicationStateReducer(ready, { type: 'delta', envelope: delta })
    expect(updated.revision).toBe(5)
    expect(updated.projection.hosts['host/a'].id).toBe('a')
    expect(applicationStateReducer(updated, { type: 'delta', envelope: delta })).toBe(updated)
  })

  it.each([
    ['revision gap', { instance_id: 'instance-a', base_revision: 2 }],
    ['instance mismatch', { instance_id: 'instance-b', base_revision: 4 }],
  ])('rehydrates on %s without changing the projection', (_name, override) => {
    const ready = applicationStateReducer(initialApplicationState, {
      type: 'snapshot',
      envelope: snapshot(),
    })
    const delta: DeltaEnvelope = {
      type: 'delta',
      schema_version: 1,
      instance_id: override.instance_id,
      base_revision: override.base_revision,
      revision: 6,
      operations: [],
    }
    const result = applicationStateReducer(ready, { type: 'delta', envelope: delta })
    expect(result.connection).toBe('rehydrating')
    expect(result.projection).toBe(ready.projection)
  })

  it('does not partially apply a delta containing an invalid operation', () => {
    const ready = applicationStateReducer(initialApplicationState, {
      type: 'snapshot',
      envelope: snapshot(),
    })
    const result = applicationStateReducer(ready, {
      type: 'delta',
      envelope: {
        type: 'delta',
        schema_version: 1,
        instance_id: 'instance-a',
        base_revision: 4,
        revision: 5,
        operations: [
          {
            kind: 'upsert-host',
            host: { key: 'host/a', id: 'a', display_name: 'a', online: true },
          },
          {
            kind: 'remove-session',
            key: '',
          },
        ],
      },
    })
    expect(result.connection).toBe('rehydrating')
    expect(result.revision).toBe(4)
    expect(result.projection.hosts).toEqual({})
  })
})

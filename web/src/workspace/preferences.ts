import { useCallback, useState } from 'react'
import { getBrandStorage, setBrandStorage } from '../lib/brandStorage'

const storageKey = 'workspace-preferences:v1'
const recentLimit = 12

export interface WorkspacePreferences {
  pinned: string[]
  recent: string[]
}

const emptyPreferences: WorkspacePreferences = { pinned: [], recent: [] }

function canonicalTargets(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const result: string[] = []
  for (const candidate of value) {
    if (typeof candidate !== 'string') continue
    const slash = candidate.indexOf('/')
    if (slash <= 0 || slash === candidate.length - 1 || result.includes(candidate)) continue
    result.push(candidate)
  }
  return result
}

export function loadWorkspacePreferences(): WorkspacePreferences {
  try {
    const stored = getBrandStorage(storageKey)
    if (!stored) return emptyPreferences
    const parsed = JSON.parse(stored) as Record<string, unknown>
    return {
      pinned: canonicalTargets(parsed.pinned),
      recent: canonicalTargets(parsed.recent).slice(0, recentLimit),
    }
  } catch {
    return emptyPreferences
  }
}

export function saveWorkspacePreferences(preferences: WorkspacePreferences): void {
  // Deliberately persist only canonical navigation targets. Terminal input and
  // Mobile Composer drafts never enter this object or localStorage payload.
  setBrandStorage(storageKey, JSON.stringify({
    pinned: canonicalTargets(preferences.pinned),
    recent: canonicalTargets(preferences.recent).slice(0, recentLimit),
  }))
}

export function useWorkspacePreferences() {
  const [preferences, setPreferences] = useState<WorkspacePreferences>(loadWorkspacePreferences)

  const update = useCallback((producer: (current: WorkspacePreferences) => WorkspacePreferences) => {
    setPreferences(current => {
      const next = producer(current)
      saveWorkspacePreferences(next)
      return next
    })
  }, [])

  const togglePin = useCallback((target: string) => update(current => ({
    ...current,
    pinned: current.pinned.includes(target)
      ? current.pinned.filter(value => value !== target)
      : [target, ...current.pinned],
  })), [update])

  const recordRecent = useCallback((target: string) => update(current => ({
    ...current,
    recent: [target, ...current.recent.filter(value => value !== target)].slice(0, recentLimit),
  })), [update])

  const forgetTarget = useCallback((target: string) => update(current => ({
    pinned: current.pinned.filter(value => value !== target),
    recent: current.recent.filter(value => value !== target),
  })), [update])

  const replaceTarget = useCallback((previous: string, next: string) => update(current => ({
    pinned: current.pinned.map(value => value === previous ? next : value),
    recent: current.recent.map(value => value === previous ? next : value),
  })), [update])

  return { ...preferences, togglePin, recordRecent, forgetTarget, replaceTarget }
}

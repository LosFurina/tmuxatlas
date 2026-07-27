import { useCallback, useEffect, useRef } from 'react'

export interface TerminalDraftStore {
  getDraft: (targetKey: string) => string
  setDraft: (targetKey: string, value: string) => void
  clearDraft: (targetKey: string) => void
}

/**
 * Page-memory-only drafts. The caller controls the store lifetime; App owns the
 * workspace store so a transient Terminal remount cannot discard drafts.
 */
export function useTerminalDrafts(): TerminalDraftStore {
  const draftsRef = useRef(new Map<string, string>())

  useEffect(() => () => {
    draftsRef.current.clear()
  }, [])

  const getDraft = useCallback((targetKey: string) => (
    draftsRef.current.get(targetKey) ?? ''
  ), [])

  const setDraft = useCallback((targetKey: string, value: string) => {
    if (value) draftsRef.current.set(targetKey, value)
    else draftsRef.current.delete(targetKey)
  }, [])

  const clearDraft = useCallback((targetKey: string) => {
    draftsRef.current.delete(targetKey)
  }, [])

  return { getDraft, setDraft, clearDraft }
}

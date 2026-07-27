import { useCallback, useEffect, useRef } from 'react'

/**
 * Page-memory-only drafts. The Map belongs to one mounted Terminal workspace,
 * is keyed by the canonical target and is deliberately never persisted.
 */
export function useTerminalDrafts() {
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

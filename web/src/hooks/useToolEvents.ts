import { useState, useEffect, useCallback } from 'react'

export interface ToolEvent {
  tool: string
  status: 'active' | 'waiting' | 'completed' | 'error'
  host?: string
  host_name?: string
  session: string
  window: number
  pane?: string
  message?: string
  timestamp: string
  auto_detected?: boolean
}

export function applyToolEvent(previous: ToolEvent[], toolEvt: ToolEvent): ToolEvent[] {
  const samePane = (event: ToolEvent) => (
    event.session === toolEvt.session &&
    event.window === toolEvt.window &&
    (event.pane || '') === (toolEvt.pane || '') &&
    (event.host || '') === (toolEvt.host || '')
  )
  const filtered = previous.filter(event => {
    if (!samePane(event)) return true
    // Completion belongs to a specific agent. A replacement hook may have
    // already reported waiting on the same pane before passive Agy departure.
    return toolEvt.status === 'completed' && event.tool !== toolEvt.tool
  })
  if (toolEvt.status === 'completed') return filtered
  // Keep auto-detected active events so they count toward agent totals;
  // hook-based active events are transient and should be cleared.
  if (toolEvt.status === 'active' && !toolEvt.auto_detected) return filtered
  return [...filtered, toolEvt]
}

export function useToolEvents() {
  const [events, setEvents] = useState<ToolEvent[]>([])

  // Fetch initial state
  const refresh = useCallback(async () => {
    try {
      const res = await fetch('/api/tool-events')
      if (res.ok) {
        const data = await res.json()
        setEvents(data || [])
      }
    } catch (err) {
      console.error('Failed to fetch tool events:', err)
    }
  }, [])

  useEffect(() => {
    refresh()
    // Periodic re-sync to catch missed WebSocket messages
    const interval = setInterval(refresh, 5000)
    return () => clearInterval(interval)
  }, [refresh])

  // Handle incoming WebSocket tool events
  const handleEvent = useCallback((evt: any) => {
    if (evt.type !== 'tool-event') return

    const toolEvt: ToolEvent = {
      tool: evt.tool,
      status: evt.status,
      host: evt.host,
      host_name: evt.host_name,
      session: evt.session,
      window: evt.window,
      pane: evt.pane,
      message: evt.message,
      timestamp: evt.timestamp,
      auto_detected: evt.auto_detected,
    }

    setEvents(prev => applyToolEvent(prev, toolEvt))
  }, [])

  // Get events for a specific session (accepts composite key: "host/name" or "name")
  const getSessionEvents = useCallback((key: string) => {
    const idx = key.indexOf('/')
    if (idx === -1) {
      // Local session — match events with no host
      return events.filter(e => e.session === key && !e.host)
    }
    const host = key.substring(0, idx)
    const name = key.substring(idx + 1)
    return events.filter(e => e.session === name && e.host === host)
  }, [events])

  // Check if a session has any "waiting" events (accepts composite key)
  const sessionNeedsAttention = useCallback((key: string) => {
    const idx = key.indexOf('/')
    if (idx === -1) {
      return events.some(e => e.session === key && !e.host && e.status === 'waiting')
    }
    const host = key.substring(0, idx)
    const name = key.substring(idx + 1)
    return events.some(e => e.session === name && e.host === host && e.status === 'waiting')
  }, [events])

  // Dismiss a specific event (clear from server and local state)
  const dismissEvent = useCallback(async (evt: ToolEvent) => {
    try {
      await fetch('/api/tool-event', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ host: evt.host || '', session: evt.session, window: evt.window, pane: evt.pane || '' }),
      })
    } catch (err) {
      console.error('Failed to dismiss event:', err)
    }
    setEvents(prev => prev.filter(
      e => !(e.session === evt.session && e.window === evt.window && (e.pane || '') === (evt.pane || '') && (e.host || '') === (evt.host || ''))
    ))
  }, [])

  // Clear all events
  const dismissAll = useCallback(async () => {
    try {
      await fetch('/api/tool-events', { method: 'DELETE' })
    } catch (err) {
      console.error('Failed to clear events:', err)
    }
    setEvents([])
  }, [])

  return { events, handleEvent, getSessionEvents, sessionNeedsAttention, dismissEvent, dismissAll, refresh }
}

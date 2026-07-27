import { useState, useEffect, useCallback, createContext, useContext, useRef } from 'react'

export interface Preferences {
  terminal: {
    font_size: number
    font_family: string
    scrollback: number
  }
  theme: string
  custom_theme: Record<string, string>
  sidebar: {
    default_collapsed: boolean
    hidden_sessions: string[]
    collapse_mode: string
  }
  default_view: string
  notifications: {
    statuses: string[]
  }
  agent_banner: {
    auto_dismiss_seconds: number
  }
  quick_switcher_shortcut: string
  sparklines_visible: boolean
  overview_refresh_interval: number
  timestamp_format: string
  lock_timeout_minutes: number
  lock_background_faster: boolean
  lock_background_minutes: number
  fullscreen_hide_alerts: boolean
}

export const defaultPreferences: Preferences = {
  terminal: {
    font_size: 13,
    font_family: 'Space Mono',
    scrollback: 5000,
  },
  theme: 'retro-blue',
  custom_theme: {},
  sidebar: {
    default_collapsed: false,
    hidden_sessions: [],
    collapse_mode: 'small',
  },
  default_view: 'overview',
  notifications: {
    statuses: ['waiting', 'error', 'completed'],
  },
  agent_banner: {
    auto_dismiss_seconds: 0,
  },
  quick_switcher_shortcut: 'ctrl+k',
  sparklines_visible: true,
  overview_refresh_interval: 5,
  timestamp_format: 'relative',
  lock_timeout_minutes: 30,
  lock_background_faster: true,
  lock_background_minutes: 10,
  fullscreen_hide_alerts: true,
}

interface PreferencesContextValue {
  prefs: Preferences
  updatePrefs: (partial: Partial<Preferences>) => Promise<void>
  loaded: boolean
  refetch: () => Promise<void>
  saveState: 'idle' | 'saving' | 'saved' | 'error'
  saveError: string | null
  retrySave: () => Promise<void>
}

export const PreferencesContext = createContext<PreferencesContextValue>({
  prefs: defaultPreferences,
  updatePrefs: async () => {},
  loaded: false,
  refetch: async () => {},
  saveState: 'idle',
  saveError: null,
  retrySave: async () => {},
})

export function usePreferencesProvider() {
  const [prefs, setPrefs] = useState<Preferences>(defaultPreferences)
  const [loaded, setLoaded] = useState(false)
  const [saveState, setSaveState] = useState<PreferencesContextValue['saveState']>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)
  const prefsRef = useRef(prefs)
  const confirmedRef = useRef(prefs)
  const queueRef = useRef<Promise<void>>(Promise.resolve())
  const updateVersionRef = useRef(0)
  const failedPayloadRef = useRef<Preferences | null>(null)

  const applyPrefs = useCallback((next: Preferences) => {
    prefsRef.current = next
    setPrefs(next)
  }, [])

  const fetchPrefs = useCallback(async () => {
    try {
      const res = await fetch('/api/preferences')
      if (!res.ok) {
        setLoaded(true)
        return // don't parse 401/error responses as prefs
      }
      const data = await res.json()
      // Validate shape before accepting
      if (data && typeof data.theme === 'string' && data.terminal) {
        confirmedRef.current = data
        applyPrefs(data)
        failedPayloadRef.current = null
        setSaveError(null)
        setSaveState('idle')
      }
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Failed to load preferences.')
    }
    setLoaded(true)
  }, [applyPrefs])

  useEffect(() => {
    fetchPrefs()
  }, [fetchPrefs])

  const enqueueSave = useCallback((payload: Preferences, version: number) => {
    const request = async () => {
      const res = await fetch('/api/preferences', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        throw new Error(`Failed to save preferences (${res.status}).`)
      }
      const saved = await res.json() as Preferences
      confirmedRef.current = saved
      failedPayloadRef.current = null
      if (version === updateVersionRef.current) {
        applyPrefs(saved)
        setSaveError(null)
        setSaveState('saved')
      }
    }

    const result = queueRef.current.then(request, request)
    queueRef.current = result.catch(() => {})
    return result.catch((error) => {
      failedPayloadRef.current = payload
      if (version === updateVersionRef.current) {
        applyPrefs(confirmedRef.current)
        setSaveState('error')
        setSaveError(error instanceof Error ? error.message : 'Failed to save preferences.')
      }
    })
  }, [applyPrefs])

  const updatePrefs = useCallback(async (partial: Partial<Preferences>) => {
    const merged = { ...prefsRef.current, ...partial }
    const version = ++updateVersionRef.current
    applyPrefs(merged)
    setSaveError(null)
    setSaveState('saving')
    await enqueueSave(merged, version)
  }, [applyPrefs, enqueueSave])

  const retrySave = useCallback(async () => {
    const payload = failedPayloadRef.current
    if (!payload) return
    const version = ++updateVersionRef.current
    applyPrefs(payload)
    setSaveError(null)
    setSaveState('saving')
    await enqueueSave(payload, version)
  }, [applyPrefs, enqueueSave])

  return {
    prefs,
    updatePrefs,
    loaded,
    refetch: fetchPrefs,
    saveState,
    saveError,
    retrySave,
  }
}

export function usePreferences() {
  return useContext(PreferencesContext)
}

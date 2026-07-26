import { lazy, Suspense, useState, useEffect, useCallback, useRef } from 'react'
import { Sidebar } from './components/Sidebar'
import { Overview } from './components/Overview'
import { QuickSwitcher } from './components/QuickSwitcher'
import { NewSessionModal } from './components/NewSessionModal'
import { TopBar } from './components/TopBar'
import { StatusBar } from './components/StatusBar'
import { Settings } from './components/Settings'
import { HelpModal } from './components/HelpModal'
import { Login } from './components/Login'
import { Setup } from './components/Setup'
import { StateConnectionNotice } from './components/StateConnectionNotice'
import { type Session, sessionKey, parseSessionKey } from './hooks/useSessions'
import type { ToolEvent } from './hooks/useToolEvents'
import { useNotifications } from './hooks/useNotifications'
import { usePushNotifications } from './hooks/usePushNotifications'
import { usePWAInstall, type PWAInstallState } from './hooks/usePWAInstall'
import { usePreferencesProvider, usePreferences, PreferencesContext } from './hooks/usePreferences'
import { useAuth } from './hooks/useAuth'
import { applyTheme } from './theme'
import { getBrandStorage, setBrandStorage } from './lib/brandStorage'
import { postRuntimeMutation } from './lib/runtimeApi'
import { ApplicationStateProvider, useApplicationState } from './state/provider'

type View = 'overview' | 'session' | 'settings' | 'setup'

const Terminal = lazy(() => import('./components/Terminal').then(module => ({ default: module.Terminal })))

function getViewFromPath(): { view: View; sessionKey: string | null } {
  if (window.location.pathname === '/settings') {
    return { view: 'settings', sessionKey: null }
  }
  if (window.location.pathname === '/setup') {
    return { view: 'setup', sessionKey: null }
  }
  // Every session URL carries its immutable host identity.
  const hostMatch = window.location.pathname.match(/^\/session\/([^/]+)\/(.+)$/)
  if (hostMatch) {
    const host = decodeURIComponent(hostMatch[1])
    const name = decodeURIComponent(hostMatch[2])
    return { view: 'session', sessionKey: `${host}/${name}` }
  }
  return { view: 'overview', sessionKey: null }
}

function AppInner({ onLogout, pwaInstall }: { onLogout?: () => void; pwaInstall: PWAInstallState }) {
  const {
    state: applicationState,
    sessions,
    hosts,
    toolEvents: allToolEvents,
    activity,
    rehydrate,
  } = useApplicationState()
  const refresh = useCallback(async () => { rehydrate() }, [rehydrate])
  const getSessionEvents = useCallback((key: string) => {
    const { host, name } = parseSessionKey(key)
    return allToolEvents.filter(event => event.host === host && event.session === name)
  }, [allToolEvents])
  const sessionNeedsAttention = useCallback((key: string) => (
    getSessionEvents(key).some(event => event.status === 'waiting')
  ), [getSessionEvents])
  const getSessionActivity = useCallback((key: string) => activity.get(key), [activity])
  const dismissEvent = useCallback(async (event: ToolEvent) => {
    await fetch('/api/tool-event', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        host: event.host || '', session: event.session,
        window: event.window, pane: event.pane || '',
      }),
    })
  }, [])
  const dismissAllEvents = useCallback(async () => {
    await fetch('/api/tool-events', { method: 'DELETE' })
  }, [])
  const { pushState, subscribe: pushSubscribe, unsubscribe: pushUnsubscribe } = usePushNotifications()
  const { processToolEvent } = useNotifications(pushState === 'subscribed')
  const [currentView, setCurrentView] = useState<View>(() => getViewFromPath().view)
  const [selectedSession, setSelectedSession] = useState<string | null>(() => getViewFromPath().sessionKey)
  const hasMultipleHosts = hosts.length > 1
  const [serverVersion, setServerVersion] = useState<string | null>(null)
  const loadedVersionRef = useRef<string | null>(null)
  const updateAvailable = loadedVersionRef.current !== null && serverVersion !== null && serverVersion !== loadedVersionRef.current
  const [quickSwitcherOpen, setQuickSwitcherOpen] = useState(false)
  const [newSessionModalOpen, setNewSessionModalOpen] = useState(false)
  const terminalContainerRef = useRef<HTMLDivElement>(null)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return getBrandStorage('sidebar-collapsed') === 'true' } catch { return false }
  })
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [terminalFullscreen, setTerminalFullscreen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [runtimeError, setRuntimeError] = useState<string | null>(null)
  const pendingSessionRef = useRef<string | null>(null)
  const { prefs } = usePreferences()

  useEffect(() => {
    const metadata = applicationState.projection.metadata?.server as
      | { version?: string }
      | undefined
    const version = metadata?.version || null
    if (!loadedVersionRef.current && version) loadedVersionRef.current = version
    setServerVersion(version)
  }, [applicationState.projection.metadata])

  const notifiedEventsRef = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (applicationState.connection !== 'ready') return
    const signatures = new Set(allToolEvents.map(event => [
      event.host, event.session, event.window, event.pane,
      event.tool, event.status, event.timestamp,
    ].join('|')))
    if (notifiedEventsRef.current) {
      for (const event of allToolEvents) {
        const signature = [
          event.host, event.session, event.window, event.pane,
          event.tool, event.status, event.timestamp,
        ].join('|')
        if (!notifiedEventsRef.current.has(signature)) processToolEvent(event)
      }
    }
    notifiedEventsRef.current = signatures
  }, [applicationState.connection, allToolEvents, processToolEvent])

  // Auto-lock: idle detection + optional background accelerator
  const lastActivityRef = useRef<number>(Date.now())
  useEffect(() => {
    if (!onLogout || !prefs.lock_timeout_minutes) return

    const idleMs = prefs.lock_timeout_minutes * 60 * 1000
    const bgMs = prefs.lock_background_faster && prefs.lock_background_minutes
      ? prefs.lock_background_minutes * 60 * 1000
      : idleMs

    // Track user activity
    const onActivity = () => { lastActivityRef.current = Date.now() }
    const events = ['keydown', 'click', 'scroll', 'touchstart', 'mousemove'] as const
    const opts: AddEventListenerOptions = { passive: true, capture: true }
    events.forEach(e => document.addEventListener(e, onActivity, opts))

    // Check idle on an interval
    const checkInterval = setInterval(() => {
      const elapsed = Date.now() - lastActivityRef.current
      const timeout = document.hidden ? bgMs : idleMs
      if (elapsed >= timeout) {
        onLogout()
      }
    }, 30_000)

    // Also check immediately when returning from background
    const onVisibilityChange = () => {
      if (!document.hidden) {
        const elapsed = Date.now() - lastActivityRef.current
        if (elapsed >= bgMs) {
          onLogout()
        }
      }
    }
    document.addEventListener('visibilitychange', onVisibilityChange)

    return () => {
      events.forEach(e => document.removeEventListener(e, onActivity, opts as EventListenerOptions))
      clearInterval(checkInterval)
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }, [onLogout, prefs.lock_timeout_minutes, prefs.lock_background_faster, prefs.lock_background_minutes])

  // Persist sidebar state
  useEffect(() => {
    setBrandStorage('sidebar-collapsed', String(sidebarCollapsed))
  }, [sidebarCollapsed])

  // Sync URL -> state on popstate (back/forward)
  useEffect(() => {
    const onPopState = () => {
      const { view, sessionKey } = getViewFromPath()
      setCurrentView(view)
      setSelectedSession(sessionKey)
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  // Navigate to a session or view (push history)
  // sessKey is either "name" (local) or "host/name" (remote)
  const navigateTo = useCallback((sessKey: string | null, view?: View) => {
    let path: string
    if (view === 'settings') {
      path = '/settings'
    } else if (view === 'setup') {
      path = '/setup'
    } else if (sessKey) {
      const { host, name } = parseSessionKey(sessKey)
      path = `/session/${encodeURIComponent(host)}/${encodeURIComponent(name)}`
    } else {
      path = '/'
    }
    if (window.location.pathname !== path) {
      window.history.pushState(null, '', path)
    }
    setSelectedSession(sessKey)
    setCurrentView(view || (sessKey ? 'session' : 'overview'))
  }, [])

  // Global keyboard shortcuts
  useEffect(() => {
    const shortcut = prefs.quick_switcher_shortcut || 'ctrl+k'
    const onKeyDown = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey

      // Quick switcher
      if (mod) {
        let match = false
        if (shortcut === 'ctrl+k' && e.key === 'k') match = true
        if (shortcut === 'ctrl+p' && e.key === 'p') match = true
        if (shortcut === 'ctrl+space' && e.key === ' ') match = true
        if (match) {
          e.preventDefault()
          setQuickSwitcherOpen(prev => !prev)
          return
        }
      }

      // Help: Cmd/Ctrl + ? or Cmd/Ctrl + / (Linux Ctrl+Shift+/ often doesn't produce '?')
      if (mod && (e.key === '?' || e.key === '/' || (e.shiftKey && e.code === 'Slash'))) {
        e.preventDefault()
        setHelpOpen(prev => !prev)
        return
      }

      // Overview: Cmd/Ctrl + H
      if (mod && e.key === 'h' && !e.shiftKey) {
        e.preventDefault()
        navigateTo(null)
        return
      }

      // Toggle sidebar: Cmd/Ctrl + \
      if (mod && e.key === '\\') {
        e.preventDefault()
        setSidebarCollapsed(c => !c)
        return
      }

      // Settings: Cmd/Ctrl + ,
      if (mod && e.key === ',') {
        e.preventDefault()
        navigateTo(null, 'settings')
        return
      }

      // Lock / Sign out: Cmd/Ctrl + L
      if (mod && e.key === 'l' && !e.shiftKey && onLogout) {
        e.preventDefault()
        onLogout()
        return
      }

      // Jump to next alert: Cmd/Ctrl + J
      if (mod && e.key === 'j' && !e.shiftKey) {
        e.preventDefault()
        const pending = allToolEvents.filter(ev => ev.status === 'waiting' || ev.status === 'error')
        if (pending.length === 0) return
        const currentIdx = selectedSession
          ? pending.findIndex(ev => (ev.host ? `${ev.host}/${ev.session}` : ev.session) === selectedSession)
          : -1
        const next = pending[(currentIdx + 1) % pending.length]
        const sessKey = next.host ? `${next.host}/${next.session}` : next.session
        navigateTo(sessKey, 'session')
        if (next.window !== undefined) {
          const { host, name } = parseSessionKey(sessKey)
          postRuntimeMutation('/api/session/select-window', {
            host_id: host, session: name, window: next.window, pane: next.pane || undefined,
          }).catch(err => setRuntimeError(err instanceof Error ? err.message : 'The session action failed.'))
        }
        return
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [prefs.quick_switcher_shortcut, navigateTo, onLogout, allToolEvents, selectedSession])

  const connected = applicationState.connection === 'ready'
    ? true
    : applicationState.connection === 'connecting'
      ? null
      : false

  // If selected session was removed, go back to overview
  // (don't bounce if we're waiting for a newly created session to appear)
  useEffect(() => {
    if (pendingSessionRef.current && sessions.some(s => sessionKey(s) === pendingSessionRef.current)) {
      pendingSessionRef.current = null
    }
    if (selectedSession && selectedSession === pendingSessionRef.current) return
    if (selectedSession && sessions.length > 0 && !sessions.find(s => sessionKey(s) === selectedSession)) {
      navigateTo(null)
    }
  }, [sessions, selectedSession, navigateTo])

  const handleSessionSelect = (session: Session) => {
    setMobileSidebarOpen(false)
    navigateTo(sessionKey(session))
  }

  const refocusTerminal = useCallback(() => {
    requestAnimationFrame(() => {
      const textarea = terminalContainerRef.current?.querySelector('textarea.xterm-helper-textarea') as HTMLTextAreaElement | null
      textarea?.focus()
    })
  }, [])

  const jumpToSession = useCallback(async (sessKey: string, windowIndex?: number, pane?: string) => {
    navigateTo(sessKey, 'session')
    if (windowIndex !== undefined) {
      const { host, name } = parseSessionKey(sessKey)
      try {
        await postRuntimeMutation('/api/session/select-window', {
          host_id: host, session: name, window: windowIndex, pane: pane || undefined,
        })
      } catch (err) {
        console.error('Failed to select window:', err)
        setRuntimeError(err instanceof Error ? err.message : 'The session action failed.')
      }
    }
    setTimeout(() => refocusTerminal(), 200)
  }, [navigateTo, refocusTerminal])

  const navigateToSettings = useCallback(() => {
    navigateTo(null, 'settings')
  }, [navigateTo])

  const closeQuickSwitcher = useCallback(() => {
    setQuickSwitcherOpen(false)
    if (selectedSession) refocusTerminal()
  }, [selectedSession, refocusTerminal])

  const handleQuickSwitch = useCallback(async (sessKey: string, windowIndex?: number) => {
    setQuickSwitcherOpen(false)
    navigateTo(sessKey)
    if (windowIndex !== undefined) {
      const { host, name } = parseSessionKey(sessKey)
      try {
        await postRuntimeMutation('/api/session/select-window', {
          host_id: host, session: name, window: windowIndex,
        })
      } catch (err) {
        console.error('Failed to select window:', err)
        setRuntimeError(err instanceof Error ? err.message : 'The session action failed.')
      }
    }
    // Refocus after navigation and window switch settle
    setTimeout(() => refocusTerminal(), 200)
  }, [navigateTo, refocusTerminal])

  const openNewSessionModal = useCallback(() => {
    setQuickSwitcherOpen(false)
    setNewSessionModalOpen(true)
  }, [])

  const handleCreateSession = useCallback(async (name: string, hostId: string) => {
    setNewSessionModalOpen(false)
    try {
      await postRuntimeMutation('/api/session/new', {
        host_id: hostId, session: name,
      })
      const sessKey = `${hostId}/${name}`
      pendingSessionRef.current = sessKey
      navigateTo(sessKey)
      await refresh()
      window.setTimeout(() => {
        if (pendingSessionRef.current === sessKey) pendingSessionRef.current = null
      }, 10_000)
      setTimeout(() => refocusTerminal(), 300)
    } catch (err) {
      console.error('Failed to create session:', err)
      setRuntimeError(err instanceof Error ? err.message : 'The session action failed.')
      pendingSessionRef.current = null
    }
  }, [navigateTo, refresh, refocusTerminal])

  const toggleFullscreen = useCallback(() => {
    setTerminalFullscreen(f => !f)
  }, [])

  // Dynamic document title
  useEffect(() => {
    if (currentView === 'session' && selectedSession) {
      const session = sessions.find(s => sessionKey(s) === selectedSession)
      const displayName = session ? session.name : parseSessionKey(selectedSession).name
      const activeWindow = session?.windows?.find(w => w.active)
      if (activeWindow) {
        document.title = `${displayName}:${activeWindow.index}:${activeWindow.name} — TmuxAtlas`
      } else {
        document.title = `${displayName} — TmuxAtlas`
      }
    } else if (currentView === 'settings') {
      document.title = 'settings — TmuxAtlas'
    } else {
      document.title = 'TmuxAtlas'
    }
  }, [currentView, selectedSession, sessions])

  // Exit fullscreen when navigating away from terminal
  useEffect(() => {
    if (currentView !== 'session') {
      setTerminalFullscreen(false)
    }
  }, [currentView])

  const showingTerminal = currentView === 'session' && !!selectedSession

  return (
    <div className="flex flex-col h-dvh w-screen bg-background text-foreground relative">
      <StateConnectionNotice
        state={applicationState.connection}
        onAuthRequired={onLogout}
      />
      {helpOpen && <HelpModal onClose={() => setHelpOpen(false)} />}
      {runtimeError && (
        <button
          type="button"
          onClick={() => setRuntimeError(null)}
          className="absolute top-3 left-1/2 -translate-x-1/2 z-[10000] max-w-[90vw] rounded border border-destructive/50 bg-destructive/15 px-4 py-2 text-sm text-foreground shadow-lg"
        >
          {runtimeError}
        </button>
      )}
      {quickSwitcherOpen && (
        <QuickSwitcher
          sessions={sessions}
          waitingEvents={allToolEvents.filter(e => e.status === 'waiting')}
          onSelect={handleQuickSwitch}
          onOverview={() => { closeQuickSwitcher(); navigateTo(null) }}
          onCreateSession={openNewSessionModal}
          onClose={closeQuickSwitcher}
        />
      )}
      {newSessionModalOpen && (
        <NewSessionModal
          hosts={hosts}
          onCreateSession={handleCreateSession}
          onClose={() => setNewSessionModalOpen(false)}
        />
      )}
      {/* TopBar - full width */}
      {(!terminalFullscreen || !prefs.fullscreen_hide_alerts) && (
        <TopBar
          currentView={currentView}
          sidebarCollapsed={sidebarCollapsed}
          onToggleCollapse={() => {
            if (window.matchMedia('(max-width: 767px)').matches) setMobileSidebarOpen(open => !open)
            else setSidebarCollapsed(c => !c)
          }}
          onOverview={() => navigateTo(null)}
          onSettings={navigateToSettings}
          onNewSession={openNewSessionModal}
          events={allToolEvents}
          connected={connected}
          onJumpToSession={jumpToSession}
          onDismiss={dismissEvent}
          onDismissAll={dismissAllEvents}
        />
      )}
      {/* Middle: Sidebar + Content */}
      <div className="flex-1 flex overflow-hidden">
        {!terminalFullscreen && (
          <Sidebar
            sessions={sessions}
            selectedSession={selectedSession}
            collapsed={sidebarCollapsed}
            collapseMode={(prefs.sidebar.collapse_mode || 'small') as 'small' | 'hidden'}
            hasMultipleHosts={hasMultipleHosts}
            onSessionSelect={handleSessionSelect}
            onSessionRenamed={(oldKey, newKey) => {
              if (selectedSession === oldKey) {
                pendingSessionRef.current = newKey
                navigateTo(newKey)
                void refresh()
                window.setTimeout(() => {
                  if (pendingSessionRef.current === newKey) pendingSessionRef.current = null
                }, 10_000)
              } else {
                void refresh()
              }
            }}
            onRuntimeError={setRuntimeError}
            getSessionEvents={getSessionEvents}
            sessionNeedsAttention={sessionNeedsAttention}
            getSessionActivity={getSessionActivity}
            mobileOpen={mobileSidebarOpen}
            onMobileClose={() => setMobileSidebarOpen(false)}
          />
        )}
        <div className="flex-1 flex flex-col overflow-hidden">
          {currentView === 'setup' ? (
            <Setup onComplete={() => navigateTo(null)} />
          ) : currentView === 'settings' ? (
            <Settings
              pushState={pushState}
              onPushSubscribe={pushSubscribe}
              onPushUnsubscribe={pushUnsubscribe}
              onLogout={onLogout}
              pwaInstall={pwaInstall}
            />
          ) : selectedSession ? (
            <div ref={terminalContainerRef} className="flex-1 flex flex-col overflow-hidden">
              <Suspense fallback={<div className="flex-1 grid place-items-center font-mono text-muted-foreground">Loading terminal…</div>}>
                <Terminal
                  sessionName={parseSessionKey(selectedSession).name}
                  hostId={parseSessionKey(selectedSession).host}
                  fullscreen={terminalFullscreen}
                  onToggleFullscreen={toggleFullscreen}
                />
              </Suspense>
            </div>
          ) : (
            <Overview
              sessions={sessions}
              hosts={hosts}
              onSessionSelect={handleSessionSelect}
              getSessionEvents={getSessionEvents}
              getSessionActivity={getSessionActivity}
              pendingAlerts={allToolEvents.filter(e => e.status === 'waiting' || e.status === 'error')}
              onJumpToSession={jumpToSession}
              onDismissAlert={dismissEvent}
            />
          )}
        </div>
      </div>
      {/* StatusBar - full width */}
      <StatusBar
        sessionCount={sessions.length}
        connected={connected}
        activeSession={selectedSession ? sessions.find(s => sessionKey(s) === selectedSession) ?? null : null}
        waitingCount={allToolEvents.filter(e => e.status === 'waiting').length}
        pushState={pushState}
        version={serverVersion}
        updateAvailable={updateAvailable}
        hosts={hosts}
        agentCount={allToolEvents.filter(e => e.auto_detected || e.status === 'waiting' || e.status === 'error').length}
        onHelp={() => setHelpOpen(true)}
      />
    </div>
  )
}

export default function App() {
  const prefsProvider = usePreferencesProvider()
  const pwaInstall = usePWAInstall()
  const {
    loading, authRequired, needsSetup, authenticated, error: authError,
    rpId, origin, setup, login, logout,
  } = useAuth()
  const [showOnboarding, setShowOnboarding] = useState(false)

  // Re-fetch preferences after login (initial fetch may have gotten 401)
  useEffect(() => {
    if (authenticated) {
      prefsProvider.refetch()
    }
  }, [authenticated]) // eslint-disable-line react-hooks/exhaustive-deps

  // Apply last-used theme immediately (before auth) so login page is themed
  useEffect(() => {
    try {
      const cached = getBrandStorage('theme')
      const cachedCustom = getBrandStorage('custom-theme')
      if (cached) {
        applyTheme(cached, cachedCustom ? JSON.parse(cachedCustom) : undefined)
      }
    } catch {}
  }, [])

  // Apply theme when preferences load or theme/customizations change, and cache for login page
  useEffect(() => {
    if (prefsProvider.loaded) {
      applyTheme(prefsProvider.prefs.theme, prefsProvider.prefs.custom_theme)
      try {
        setBrandStorage('theme', prefsProvider.prefs.theme)
        setBrandStorage('custom-theme', JSON.stringify(prefsProvider.prefs.custom_theme || {}))
      } catch {}
    }
  }, [prefsProvider.loaded, prefsProvider.prefs.theme, prefsProvider.prefs.custom_theme])

  if (loading) {
    return <div className="flex items-center justify-center h-dvh w-screen bg-background" />
  }

  if (authRequired && needsSetup) {
    const handleSetup = async (setupToken: string, label?: string) => {
      const ok = await setup(setupToken, label)
      if (ok) setShowOnboarding(true)
      return ok
    }
    return <Login mode="setup" error={authError} rpId={rpId} origin={origin} onSubmit={handleSetup} />
  }

  if (authRequired && !authenticated) {
    return <Login mode="login" error={authError} rpId={rpId} origin={origin} onSubmit={() => login()} />
  }

  if (authenticated && showOnboarding) {
    return (
      <PreferencesContext.Provider value={prefsProvider}>
        <Setup fullPage onComplete={() => {
          setShowOnboarding(false)
          try { setBrandStorage('setup-seen', 'true') } catch {}
        }} />
      </PreferencesContext.Provider>
    )
  }

  return (
    <PreferencesContext.Provider value={prefsProvider}>
      <ApplicationStateProvider>
        <AppInner onLogout={authRequired ? logout : undefined} pwaInstall={pwaInstall} />
      </ApplicationStateProvider>
    </PreferencesContext.Provider>
  )
}

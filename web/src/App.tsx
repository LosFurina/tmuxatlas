import { lazy, Suspense, useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { Sidebar } from './components/Sidebar'
import { TopBar } from './components/TopBar'
import { StatusBar } from './components/StatusBar'
import { Login } from './components/Login'
import { StateConnectionNotice } from './components/StateConnectionNotice'
import {
  AuthLoadingState,
  EmptyWorkspaceState,
  WorkspaceLoadingState,
} from './components/PageStates'
import type { TerminalCommandActions } from './components/Terminal'
import { useTerminalDrafts } from './hooks/useTerminalDrafts'
import { type Session, sessionKey, parseSessionKey } from './hooks/useSessions'
import type { ToolEvent } from './hooks/useToolEvents'
import { useNotifications } from './hooks/useNotifications'
import { usePushNotifications } from './hooks/usePushNotifications'
import { usePWAInstall, type PWAInstallState } from './hooks/usePWAInstall'
import { usePreferencesProvider, usePreferences, PreferencesContext } from './hooks/usePreferences'
import { useVisualViewportVariables } from './hooks/useVisualViewport'
import { useAuth } from './hooks/useAuth'
import { applyTheme } from './theme'
import { getBrandStorage, setBrandStorage } from './lib/brandStorage'
import { postRuntimeMutation } from './lib/runtimeApi'
import { ApplicationStateProvider, useApplicationState } from './state/provider'
import { createCommandRegistry, dispatchCommandShortcut, type CommandHandler, type CommandId, type CommandScope } from './commands/registry'
import { buildWorkspaceViewModel } from './workspace/model'
import { useWorkspacePreferences } from './workspace/preferences'

type View = 'overview' | 'session' | 'settings' | 'setup'

const HelpModal = lazy(() => import('./components/HelpModal').then(module => ({ default: module.HelpModal })))
const NewSessionModal = lazy(() => import('./components/NewSessionModal').then(module => ({ default: module.NewSessionModal })))
const Overview = lazy(() => import('./components/Overview').then(module => ({ default: module.Overview })))
const QuickSwitcher = lazy(() => import('./components/QuickSwitcher').then(module => ({ default: module.QuickSwitcher })))
const Settings = lazy(() => import('./components/Settings').then(module => ({ default: module.Settings })))
const Setup = lazy(() => import('./components/Setup').then(module => ({ default: module.Setup })))
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
  const [serverVersion, setServerVersion] = useState<string | null>(null)
  const loadedVersionRef = useRef<string | null>(null)
  const updateAvailable = loadedVersionRef.current !== null && serverVersion !== null && serverVersion !== loadedVersionRef.current
  const [quickSwitcherOpen, setQuickSwitcherOpen] = useState(false)
  const [newSessionModalOpen, setNewSessionModalOpen] = useState(false)
  const terminalContainerRef = useRef<HTMLDivElement>(null)
  const terminalActionsRef = useRef<TerminalCommandActions | null>(null)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [terminalFullscreen, setTerminalFullscreen] = useState(false)
  const [zenMode, setZenMode] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [runtimeError, setRuntimeError] = useState<string | null>(null)
  const pendingSessionRef = useRef<string | null>(null)
  const sidebarDefaultAppliedRef = useRef(false)
  const defaultViewAppliedRef = useRef(false)
  const { prefs, loaded: preferencesLoaded } = usePreferences()
  const workspacePreferences = useWorkspacePreferences()
  const terminalDrafts = useTerminalDrafts()
  const workspace = useMemo(() => buildWorkspaceViewModel(sessions, allToolEvents, activity, hosts), [sessions, allToolEvents, activity, hosts])

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

  // Server-backed Preferences are the source of truth for the initial shell.
  // Toggling the Sidebar remains a per-page action; Settings changes the
  // default used by the next page load.
  useEffect(() => {
    if (!preferencesLoaded || sidebarDefaultAppliedRef.current) return
    sidebarDefaultAppliedRef.current = true
    setSidebarCollapsed(Boolean(prefs.sidebar.default_collapsed))
  }, [preferencesLoaded, prefs.sidebar.default_collapsed])

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

  useEffect(() => {
    if (
      defaultViewAppliedRef.current
      || !preferencesLoaded
      || applicationState.connection !== 'ready'
    ) {
      return
    }
    defaultViewAppliedRef.current = true
    if (window.location.pathname !== '/' || prefs.default_view !== 'last-session') return

    const target = workspacePreferences.recent.find(candidate => workspace.byTarget.has(candidate))
    if (!target) return
    const { host, name } = parseSessionKey(target)
    window.history.replaceState(
      null,
      '',
      `/session/${encodeURIComponent(host)}/${encodeURIComponent(name)}`,
    )
    setSelectedSession(target)
    setCurrentView('session')
  }, [
    applicationState.connection,
    preferencesLoaded,
    prefs.default_view,
    workspace.byTarget,
    workspacePreferences.recent,
  ])

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
    const target = sessionKey(session)
    workspacePreferences.recordRecent(target)
    navigateTo(target)
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
    workspacePreferences.recordRecent(sessKey)
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
  }, [navigateTo, refocusTerminal, workspacePreferences])

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
      workspacePreferences.recordRecent(sessKey)
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
  }, [navigateTo, refresh, refocusTerminal, workspacePreferences])

  const toggleFullscreen = useCallback(() => {
    setTerminalFullscreen(f => !f)
    setTimeout(() => refocusTerminal(), 0)
  }, [refocusTerminal])

  const toggleZen = useCallback(() => {
    setZenMode(value => !value)
    setTimeout(() => refocusTerminal(), 0)
  }, [refocusTerminal])

  const goToNextAlert = useCallback(() => {
    const pending = allToolEvents.filter(event => event.status === 'waiting' || event.status === 'error')
    if (pending.length === 0) return
    const currentIndex = selectedSession
      ? pending.findIndex(event => `${event.host}/${event.session}` === selectedSession)
      : -1
    const next = pending[(currentIndex + 1) % pending.length]
    const target = `${next.host}/${next.session}`
    workspacePreferences.recordRecent(target)
    void jumpToSession(target, next.window, next.pane)
  }, [allToolEvents, jumpToSession, selectedSession, workspacePreferences])

  const showingTerminal = currentView === 'session' && !!selectedSession
  const activeSession = selectedSession
    ? sessions.find(session => sessionKey(session) === selectedSession) ?? null
    : null
  const activeHost = activeSession
    ? hosts.find(host => host.id === activeSession.host) ?? null
    : null
  const activeWindow = activeSession?.windows.find(window => window.active)
  const activePane = activeWindow?.panes.find(pane => pane.active)
  const activeToolEvent = selectedSession
    ? getSessionEvents(selectedSession).find(event => event.status === 'waiting' || event.status === 'error')
      ?? getSessionEvents(selectedSession)[0]
    : undefined
  const retryTerminal = useCallback(() => {
    rehydrate()
    terminalActionsRef.current?.reconnect()
  }, [rehydrate])
  const handleTerminalCommandActionsChange = useCallback((actions: TerminalCommandActions | null) => {
    terminalActionsRef.current = actions
  }, [])

  const commandHandlers = useMemo<Partial<Record<CommandId, CommandHandler>>>(() => ({
    'palette.open': () => setQuickSwitcherOpen(value => !value),
    'help.open': () => {
      setQuickSwitcherOpen(false)
      setHelpOpen(value => !value)
    },
    'navigation.overview': () => navigateTo(null),
    'navigation.settings': navigateToSettings,
    'session.new': openNewSessionModal,
    'connection.reconnect': () => terminalActionsRef.current?.reconnect(),
    'terminal.fullscreen': toggleFullscreen,
    'terminal.zen': toggleZen,
    'workspace.sidebar.toggle': () => setSidebarCollapsed(value => !value),
    'attention.next': goToNextAlert,
    ...(onLogout ? { 'account.sign-out': onLogout } : {}),
  }), [goToNextAlert, navigateTo, navigateToSettings, onLogout, openNewSessionModal, toggleFullscreen, toggleZen])

  const commands = useMemo(() => createCommandRegistry({
    environment: {
      hasTerminalTarget: showingTerminal,
      canReconnect: showingTerminal,
      canSignOut: Boolean(onLogout),
      hasAttention: allToolEvents.some(event => event.status === 'waiting' || event.status === 'error'),
    },
    handlers: commandHandlers,
    quickSwitcherShortcut: prefs.quick_switcher_shortcut,
  }), [allToolEvents, commandHandlers, onLogout, prefs.quick_switcher_shortcut, showingTerminal])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const overlayOpen = quickSwitcherOpen || helpOpen || newSessionModalOpen || mobileSidebarOpen
      let scope: CommandScope = overlayOpen ? 'overlay' : 'workspace'
      if (!overlayOpen && showingTerminal && terminalContainerRef.current?.contains(document.activeElement)) scope = 'terminal'
      dispatchCommandShortcut(event, commands, scope)
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [commands, helpOpen, mobileSidebarOpen, newSessionModalOpen, quickSwitcherOpen, showingTerminal])

  const detachBrowserSession = useCallback((target: string) => {
    // This intentionally changes only browser navigation. It never calls a
    // tmux lifecycle mutation, so the underlying Session keeps running.
    if (selectedSession === target) navigateTo(null)
    setMobileSidebarOpen(false)
  }, [navigateTo, selectedSession])

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
      setZenMode(false)
    }
  }, [currentView])

  if (
    applicationState.instanceId === null
    && (applicationState.connection === 'connecting' || applicationState.connection === 'rehydrating')
  ) {
    return (
      <>
        <StateConnectionNotice state={applicationState.connection} onAuthRequired={onLogout} />
        <WorkspaceLoadingState />
      </>
    )
  }

  return (
    <div className="app-shell flex flex-col bg-background text-foreground">
      {(!zenMode
        || applicationState.connection === 'auth-required'
        || applicationState.connection === 'reload-required') && (
        <StateConnectionNotice
          state={applicationState.connection}
          onAuthRequired={onLogout}
        />
      )}
      {helpOpen && (
        <Suspense fallback={null}>
          <HelpModal commands={commands} onClose={() => setHelpOpen(false)} />
        </Suspense>
      )}
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
        <Suspense fallback={null}>
          <QuickSwitcher
            workspace={workspace}
            commands={commands}
            pinnedTargets={workspacePreferences.pinned}
            recentTargets={workspacePreferences.recent}
            onSelect={handleQuickSwitch}
            onClose={closeQuickSwitcher}
          />
        </Suspense>
      )}
      {newSessionModalOpen && (
        <Suspense fallback={null}>
          <NewSessionModal
            hosts={hosts}
            onCreateSession={handleCreateSession}
            onClose={() => setNewSessionModalOpen(false)}
          />
        </Suspense>
      )}
      {/* TopBar - full width */}
      {!zenMode && (!terminalFullscreen || !prefs.fullscreen_hide_alerts) && (
        <TopBar
          currentView={currentView}
          sidebarCollapsed={sidebarCollapsed}
          onToggleCollapse={() => {
            if (window.matchMedia('(max-width: 1023px)').matches) setMobileSidebarOpen(open => !open)
            else setSidebarCollapsed(c => !c)
          }}
          onOverview={() => navigateTo(null)}
          onSettings={navigateToSettings}
          onNewSession={openNewSessionModal}
          events={allToolEvents}
          onJumpToSession={jumpToSession}
          onDismiss={dismissEvent}
          onDismissAll={dismissAllEvents}
        />
      )}
      {/* Middle: Sidebar + Content */}
      <div className="flex-1 flex overflow-hidden">
        {!terminalFullscreen && !zenMode && (
          <Sidebar
            workspace={workspace}
            selectedSession={selectedSession}
            collapsed={sidebarCollapsed}
            collapseMode={(prefs.sidebar.collapse_mode || 'small') as 'small' | 'hidden'}
            pinnedTargets={workspacePreferences.pinned}
            recentTargets={workspacePreferences.recent}
            onTogglePin={workspacePreferences.togglePin}
            onSessionSelect={handleSessionSelect}
            onDetachSession={detachBrowserSession}
            onSessionRenamed={(oldKey, newKey) => {
              workspacePreferences.replaceTarget(oldKey, newKey)
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
            mobileOpen={mobileSidebarOpen}
            onMobileClose={() => setMobileSidebarOpen(false)}
          />
        )}
        <div className="flex-1 flex flex-col overflow-hidden">
          <Suspense fallback={<div role="status" className="flex-1 grid place-items-center text-sm text-muted-foreground">Loading view…</div>}>
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
                <Terminal
                  sessionName={parseSessionKey(selectedSession).name}
                  hostId={parseSessionKey(selectedSession).host}
                  hostName={activeHost?.name || activeSession?.host_name}
                  windowLabel={activeWindow ? `${activeWindow.index}:${activeWindow.name}` : undefined}
                  paneLabel={activePane ? String(activePane.index) : undefined}
                  toolStatus={activeToolEvent ? `${activeToolEvent.tool} · ${activeToolEvent.status}` : undefined}
                  hubConnection={applicationState.connection}
                  agentOnline={activeSession ? (activeHost?.online ?? activeSession.host_online !== false) : true}
                  sessionAvailable={Boolean(activeSession)}
                  fullscreen={terminalFullscreen}
                  zenMode={zenMode}
                  onToggleFullscreen={toggleFullscreen}
                  onToggleZen={toggleZen}
                  onRetry={retryTerminal}
                  onCommandActionsChange={handleTerminalCommandActionsChange}
                  terminalDrafts={terminalDrafts}
                />
              </div>
            ) : applicationState.connection === 'ready' && sessions.length === 0 ? (
              <EmptyWorkspaceState
                onNewSession={openNewSessionModal}
                onOpenSetup={() => navigateTo(null, 'setup')}
              />
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
          </Suspense>
        </div>
      </div>
      {/* StatusBar - full width */}
      {!zenMode && <StatusBar
        sessionCount={sessions.length}
        waitingCount={allToolEvents.filter(e => e.status === 'waiting').length}
        pushState={pushState}
        version={serverVersion}
        updateAvailable={updateAvailable}
        hosts={hosts}
        agentCount={allToolEvents.filter(e => e.auto_detected || e.status === 'waiting' || e.status === 'error').length}
        onHelp={() => setHelpOpen(true)}
      />}
    </div>
  )
}

export default function App() {
  useVisualViewportVariables()
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
    return <AuthLoadingState />
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
        <Suspense fallback={<WorkspaceLoadingState label="Loading setup" />}>
          <Setup fullPage onComplete={() => {
            setShowOnboarding(false)
            try { setBrandStorage('setup-seen', 'true') } catch {}
          }} />
        </Suspense>
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

import { useCallback, useEffect, useRef, useState } from 'react'
import {
  useTerminal,
  type PendingTerminalPaste,
} from '../hooks/useTerminal'
import { useTerminalDrafts } from '../hooks/useTerminalDrafts'
import {
  MobileTerminalInput,
  terminalKeys,
  type ModifierName,
  type ModifierState,
} from '../lib/mobileTerminalInput'
import { terminalTargetKey } from '../lib/terminalInput'
import type { ConnectionState } from '../state/types'
import {
  deriveTerminalWorkspaceConnection,
  type TerminalWorkspaceConnectionState,
} from '../state/terminalConnection'
import { TerminalCockpit } from './TerminalCockpit'
import { TerminalContextMenu, type TerminalMenuPosition } from './TerminalContextMenu'
import { TerminalPasteConfirmation } from './TerminalPasteConfirmation'
import { TerminalSearchBar } from './TerminalSearchBar'
import { MobileInputComposer } from './MobileInputComposer'

export interface TerminalCommandActions {
  reconnect: () => void
  focus: () => void
  openSearch: () => void
  copy: () => Promise<void>
  paste: () => Promise<void>
  scrollToBottom: () => void
}

export interface TerminalProps {
  sessionName: string
  hostId: string
  hostName?: string
  windowLabel?: string
  paneLabel?: string
  toolStatus?: string
  hubConnection?: ConnectionState
  agentOnline?: boolean
  sessionAvailable?: boolean
  connectionState?: TerminalWorkspaceConnectionState
  fullscreen?: boolean
  zenMode?: boolean
  onToggleFullscreen?: () => void
  onToggleZen?: () => void
  onRetry?: () => void
  onCommandActionsChange?: (actions: TerminalCommandActions | null) => void
}

const terminalStateMessages: Record<TerminalWorkspaceConnectionState, string> = {
  connecting: 'Connecting to Terminal…',
  rehydrating: 'Synchronizing Hub state…',
  connected: '',
  reconnecting: 'Terminal disconnected. Reconnecting…',
  'hub-offline': 'The Hub connection is unavailable.',
  'agent-offline': 'The target Agent is offline.',
  'session-ended': 'This tmux Session is no longer available.',
  'auth-required': 'Your Hub login has expired.',
  'reload-required': 'TmuxAtlas was updated. Reload this page to continue.',
}

export function Terminal({
  sessionName,
  hostId,
  hostName,
  windowLabel,
  paneLabel,
  toolStatus,
  hubConnection,
  agentOnline,
  sessionAvailable,
  connectionState,
  fullscreen,
  zenMode,
  onToggleFullscreen,
  onToggleZen,
  onRetry,
  onCommandActionsChange,
}: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<MobileTerminalInput | null>(null)
  if (!inputRef.current) inputRef.current = new MobileTerminalInput()
  const input = inputRef.current
  const targetKey = terminalTargetKey(hostId, sessionName)
  const targetLabel = `${hostName || hostId}/${sessionName}`
  const { getDraft, setDraft } = useTerminalDrafts()
  const [modifiers, setModifiers] = useState<ModifierState>(input.snapshot())
  const [toolbarOpen, setToolbarOpen] = useState(true)
  const [toolbarError, setToolbarError] = useState('')
  const [toolbarFeedback, setToolbarFeedback] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [contextMenu, setContextMenu] = useState<TerminalMenuPosition | null>(null)
  const [pendingPaste, setPendingPaste] = useState<PendingTerminalPaste | null>(null)
  const [pasteError, setPasteError] = useState('')
  const {
    connect,
    disconnect,
    reconnect,
    fit,
    focus,
    ptyState,
    termConnected,
    hasSelection,
    isAtBottom,
    hasNewOutput,
    scrollToBottom,
    adjustFontSize,
    sendInput,
    sendCommand,
    copySelection,
    pasteClipboard,
    commitClipboardPaste,
    selectAll,
    searchState,
    ensureSearchAddon,
    findNext,
    findPrevious,
    clearSearch,
  } = useTerminal(sessionName, hostId, input)

  const workspaceConnection = connectionState ?? deriveTerminalWorkspaceConnection({
    hub: hubConnection ?? 'ready',
    agentOnline: agentOnline ?? true,
    sessionAvailable: sessionAvailable ?? true,
    pty: ptyState,
  }).state
  const canPaste = workspaceConnection === 'connected' && Boolean(navigator.clipboard?.readText)

  useEffect(() => input.subscribe(setModifiers), [input])
  useEffect(() => {
    input.reset()
    setToolbarError('')
    setToolbarFeedback('')
    setPendingPaste(null)
    setPasteError('')
    setContextMenu(null)
    setSearchOpen(false)
    clearSearch()
  }, [clearSearch, hostId, input, sessionName])

  const cycleModifier = (modifier: ModifierName) => {
    input.cycle(modifier)
    focus()
  }

  const reportError = useCallback((error: unknown, fallback: string) => {
    setToolbarFeedback('')
    setToolbarError(error instanceof Error ? error.message : fallback)
  }, [])

  const copy = useCallback(async () => {
    setToolbarError('')
    try {
      await copySelection()
      setToolbarFeedback('Copied Terminal selection.')
    } catch (error) {
      reportError(error, 'The selection could not be copied.')
      throw error
    }
  }, [copySelection, reportError])

  const paste = useCallback(async () => {
    setToolbarError('')
    setToolbarFeedback('')
    try {
      const confirmation = await pasteClipboard()
      if (confirmation) {
        setPendingPaste(confirmation)
        setPasteError('')
      } else {
        setToolbarFeedback('Pasted into Terminal.')
      }
    } catch (error) {
      reportError(error, 'Clipboard paste failed.')
      throw error
    }
  }, [pasteClipboard, reportError])

  const confirmPaste = useCallback(() => {
    if (!pendingPaste) return
    setPasteError('')
    try {
      commitClipboardPaste(pendingPaste)
      setPendingPaste(null)
      setToolbarFeedback('Pasted multiple lines into Terminal.')
      setToolbarError('')
    } catch (error) {
      setPasteError(error instanceof Error ? error.message : 'The paste could not be sent.')
    }
  }, [commitClipboardPaste, pendingPaste])

  const openSearch = useCallback(() => {
    setContextMenu(null)
    setSearchOpen(true)
  }, [])

  const closeSearch = useCallback(() => {
    clearSearch()
    setSearchOpen(false)
    window.requestAnimationFrame(() => focus())
  }, [clearSearch, focus])

  const sendComposerCommand = useCallback((value: string) => {
    sendCommand(value)
    setToolbarError('')
  }, [sendCommand])

  useEffect(() => {
    if (!onCommandActionsChange) return
    const actions: TerminalCommandActions = {
      reconnect,
      focus,
      openSearch,
      copy: async () => { await copy() },
      paste: async () => { await paste() },
      scrollToBottom,
    }
    onCommandActionsChange(actions)
    return () => onCommandActionsChange(null)
  }, [
    copy,
    focus,
    onCommandActionsChange,
    openSearch,
    paste,
    reconnect,
    scrollToBottom,
    targetKey,
  ])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    connect(container)
    const focusFrame = window.requestAnimationFrame(() => focus())
    const focusTimer = window.setTimeout(() => focus(), 100)
    return () => {
      window.cancelAnimationFrame(focusFrame)
      window.clearTimeout(focusTimer)
      disconnect()
    }
  }, [connect, disconnect, focus])

  useEffect(() => {
    if (!termConnected || document.hidden) return
    const timer = window.setTimeout(() => {
      fit()
      focus()
      containerRef.current
        ?.querySelector<HTMLTextAreaElement>('textarea.xterm-helper-textarea')
        ?.focus()
    }, 100)
    return () => window.clearTimeout(timer)
  }, [fit, focus, termConnected])

  useEffect(() => {
    let timer: number | null = null
    const refocus = () => {
      if (document.hidden || !containerRef.current) return
      if (timer !== null) window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        timer = null
        fit()
        focus()
        containerRef.current
          ?.querySelector<HTMLTextAreaElement>('textarea.xterm-helper-textarea')
          ?.focus()
      }, 200)
    }
    document.addEventListener('visibilitychange', refocus)
    window.addEventListener('focus', refocus)
    return () => {
      document.removeEventListener('visibilitychange', refocus)
      window.removeEventListener('focus', refocus)
      if (timer !== null) window.clearTimeout(timer ?? undefined)
    }
  }, [fit, focus])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    let frame: number | null = null
    const scheduleFit = () => {
      if (frame !== null) window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(() => {
        frame = null
        fit()
      })
    }
    const observer = new ResizeObserver(scheduleFit)
    observer.observe(container)
    window.visualViewport?.addEventListener('resize', scheduleFit)
    window.visualViewport?.addEventListener('scroll', scheduleFit)
    return () => {
      observer.disconnect()
      if (frame !== null) window.cancelAnimationFrame(frame)
      window.visualViewport?.removeEventListener('resize', scheduleFit)
      window.visualViewport?.removeEventListener('scroll', scheduleFit)
    }
  }, [fit])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    let lastY = 0
    let lastTime = 0
    let accumulated = 0
    let velocity = 0
    let inertiaId: number | null = null
    let lastClientX = 0
    let lastClientY = 0
    const lineHeight = 20

    const dispatchScroll = (lines: number, clientX: number, clientY: number) => {
      const target = container.querySelector('.xterm-screen')
      if (!target) return
      for (let index = 0; index < Math.abs(lines); index++) {
        target.dispatchEvent(new WheelEvent('wheel', {
          deltaY: lines > 0 ? lineHeight : -lineHeight,
          clientX,
          clientY,
          bubbles: true,
          cancelable: true,
        }))
      }
    }

    const processAccumulated = (clientX: number, clientY: number) => {
      const speed = Math.abs(velocity)
      const multiplier = speed > 12 ? 8 : speed > 7 ? 5 : speed > 4 ? 3 : speed > 2 ? 2 : 1
      const threshold = lineHeight / multiplier
      while (Math.abs(accumulated) >= threshold) {
        const direction = accumulated > 0 ? 1 : -1
        dispatchScroll(direction, clientX, clientY)
        accumulated -= direction * threshold
      }
    }

    const stopInertia = () => {
      if (inertiaId === null) return
      window.cancelAnimationFrame(inertiaId)
      inertiaId = null
    }
    const inertiaLoop = () => {
      if (Math.abs(velocity) < 0.3) {
        velocity = 0
        accumulated = 0
        inertiaId = null
        return
      }
      velocity *= 0.92
      accumulated += velocity * 16
      processAccumulated(lastClientX, lastClientY)
      inertiaId = window.requestAnimationFrame(inertiaLoop)
    }
    const onTouchStart = (event: TouchEvent) => {
      if (event.touches.length !== 1) return
      stopInertia()
      lastY = event.touches[0].clientY
      lastTime = performance.now()
      accumulated = 0
      velocity = 0
    }
    const onTouchMove = (event: TouchEvent) => {
      if (event.touches.length !== 1) return
      event.preventDefault()
      const now = performance.now()
      const currentY = event.touches[0].clientY
      const deltaY = lastY - currentY
      const elapsed = now - lastTime
      lastClientX = event.touches[0].clientX
      lastClientY = event.touches[0].clientY
      if (elapsed > 0) velocity = velocity * 0.3 + (deltaY / elapsed) * 0.7
      accumulated += deltaY
      lastY = currentY
      lastTime = now
      processAccumulated(lastClientX, lastClientY)
    }
    const onTouchEnd = () => {
      if (Math.abs(velocity) > 0.5) inertiaId = window.requestAnimationFrame(inertiaLoop)
    }

    container.addEventListener('touchstart', onTouchStart, { passive: true })
    container.addEventListener('touchmove', onTouchMove, { passive: false })
    container.addEventListener('touchend', onTouchEnd, { passive: true })
    container.addEventListener('touchcancel', onTouchEnd, { passive: true })
    return () => {
      stopInertia()
      container.removeEventListener('touchstart', onTouchStart)
      container.removeEventListener('touchmove', onTouchMove)
      container.removeEventListener('touchend', onTouchEnd)
      container.removeEventListener('touchcancel', onTouchEnd)
    }
  }, [sessionName])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      fit()
      focus()
    }, 100)
    return () => window.clearTimeout(timer)
  }, [fit, focus, fullscreen, zenMode])

  const showMenuAtButton = (button: HTMLButtonElement) => {
    const rect = button.getBoundingClientRect()
    setContextMenu({ x: rect.right - 160, y: rect.bottom + 4 })
  }

  return (
    <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden p-1">
      {!zenMode && (
        <TerminalCockpit
          hostLabel={hostName || hostId}
          sessionName={sessionName}
          windowLabel={windowLabel}
          paneLabel={paneLabel}
          toolStatus={toolStatus}
          connectionState={workspaceConnection}
          canCopy={hasSelection}
          canPaste={canPaste}
          showScrollToBottom={!isAtBottom || hasNewOutput}
          fullscreen={fullscreen}
          zenMode={zenMode}
          onSearch={openSearch}
          onCopy={() => void copy().catch(() => {})}
          onPaste={() => void paste().catch(() => {})}
          onFontSize={adjustFontSize}
          onScrollToBottom={scrollToBottom}
          onToggleFullscreen={onToggleFullscreen}
          onToggleZen={onToggleZen}
          onMore={showMenuAtButton}
        />
      )}
      {zenMode && onToggleZen && (
        <button
          type="button"
          aria-label="Exit Terminal Zen Mode"
          onClick={onToggleZen}
          className="absolute right-3 top-3 z-20 min-h-11 rounded border border-border bg-card/95 px-3 text-xs text-muted-foreground"
        >
          Exit Zen
        </button>
      )}
      {searchOpen && (
        <TerminalSearchBar
          state={searchState}
          onLoad={ensureSearchAddon}
          onFindNext={findNext}
          onFindPrevious={findPrevious}
          onRetry={ensureSearchAddon}
          onClose={closeSearch}
        />
      )}
      <div
        className="relative min-h-0 w-full flex-1 rounded border border-border bg-card"
        style={{ boxShadow: 'var(--terminal-chrome-shadow)' }}
        onContextMenu={event => {
          event.preventDefault()
          setContextMenu({ x: event.clientX, y: event.clientY })
        }}
      >
        <div
          ref={containerRef}
          data-terminal-surface
          className="absolute inset-0.5 overflow-hidden rounded-sm"
        />
        {workspaceConnection !== 'connected' && (
          <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded bg-background/80">
            <div className="pointer-events-auto flex max-w-[90%] items-center gap-3 rounded border border-border bg-card px-4 py-3 font-mono text-sm">
              <span className="h-2 w-2 shrink-0 animate-[pulse_1.5s_ease-in-out_infinite] rounded-full bg-warning" />
              <span>{terminalStateMessages[workspaceConnection]}</span>
              {(workspaceConnection === 'reconnecting' || workspaceConnection === 'hub-offline') && (
                <button
                  type="button"
                  onClick={onRetry || reconnect}
                  className="min-h-9 rounded border border-primary/50 px-3 text-primary"
                >
                  Retry
                </button>
              )}
            </div>
          </div>
        )}
        {pendingPaste && (
          <TerminalPasteConfirmation
            text={pendingPaste.text}
            targetLabel={targetLabel}
            error={pasteError}
            onConfirm={confirmPaste}
            onCancel={() => {
              setPendingPaste(null)
              setPasteError('')
              focus()
            }}
          />
        )}
      </div>

      <div className="shrink-0 pb-[var(--safe-area-inset-bottom)] lg:hidden lg:pointer-coarse:block">
        <button
          type="button"
          className="mt-1 min-h-11 w-full rounded border border-border bg-card text-xs font-mono"
          aria-expanded={toolbarOpen}
          aria-controls="mobile-terminal-toolbar"
          onClick={() => setToolbarOpen(value => !value)}
        >
          {toolbarOpen ? 'Hide terminal keys' : 'Show terminal keys'}
        </button>
        {toolbarOpen && (
          <div
            id="mobile-terminal-toolbar"
            className="mt-1 flex gap-1 overflow-x-auto overscroll-x-contain"
            role="toolbar"
            aria-label="Terminal keys"
          >
            {([
              ['Esc', terminalKeys.escape],
              ['Tab', terminalKeys.tab],
              ['←', terminalKeys.left],
              ['↑', terminalKeys.up],
              ['↓', terminalKeys.down],
              ['→', terminalKeys.right],
            ] as const).map(([label, value]) => (
              <button
                key={label}
                type="button"
                aria-label={
                  label === '←' ? 'Left arrow'
                    : label === '↑' ? 'Up arrow'
                      : label === '↓' ? 'Down arrow'
                        : label === '→' ? 'Right arrow'
                          : label
                }
                className="min-h-11 min-w-11 rounded border border-border bg-card font-mono"
                onClick={() => {
                  sendInput(value)
                  focus()
                }}
              >
                {label}
              </button>
            ))}
            {(['ctrl', 'alt'] as const).map(modifier => (
              <button
                key={modifier}
                type="button"
                aria-label={`${modifier === 'ctrl' ? 'Control' : 'Alt'} modifier: ${modifiers[modifier]}`}
                aria-pressed={modifiers[modifier] !== 'off'}
                className="min-h-11 min-w-14 rounded border border-border bg-card font-mono data-[active=true]:border-primary data-[active=true]:text-primary"
                data-active={modifiers[modifier] !== 'off'}
                onClick={() => cycleModifier(modifier)}
              >
                {modifier === 'ctrl' ? 'Ctrl' : 'Alt'}
                {modifiers[modifier] === 'locked' ? ' 🔒' : modifiers[modifier] === 'once' ? ' ·' : ''}
              </button>
            ))}
            <button
              type="button"
              aria-label="Copy Terminal selection"
              disabled={!hasSelection}
              className="min-h-11 min-w-14 rounded border border-border bg-card font-mono disabled:opacity-40"
              onClick={() => void copy().catch(() => {})}
            >
              Copy
            </button>
            <button
              type="button"
              aria-label="Paste Clipboard into Terminal"
              disabled={!canPaste}
              className="min-h-11 min-w-14 rounded border border-border bg-card font-mono disabled:opacity-40"
              onClick={() => void paste().catch(() => {})}
            >
              Paste
            </button>
            <button
              type="button"
              aria-label="Show software keyboard"
              className="min-h-11 min-w-14 rounded border border-border bg-card font-mono"
              onClick={focus}
            >
              ⌨︎
            </button>
          </div>
        )}
        <MobileInputComposer
          key={targetKey}
          targetKey={targetKey}
          targetLabel={targetLabel}
          initialDraft={getDraft(targetKey)}
          onDraftChange={setDraft}
          onSend={sendComposerCommand}
        />
        {toolbarError && <p className="px-2 pt-1 text-xs text-destructive" role="alert">{toolbarError}</p>}
        {toolbarFeedback && <p className="px-2 pt-1 text-xs text-success" role="status">{toolbarFeedback}</p>}
      </div>

      {contextMenu && (
        <TerminalContextMenu
          position={contextMenu}
          canCopy={hasSelection}
          canPaste={canPaste}
          onCopy={() => void copy().catch(() => {})}
          onPaste={() => void paste().catch(() => {})}
          onFind={openSearch}
          onSelectAll={() => {
            selectAll()
            focus()
          }}
          onClose={() => {
            setContextMenu(null)
            focus()
          }}
        />
      )}
    </div>
  )
}

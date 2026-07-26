import { useEffect, useRef, useState, useCallback } from 'react'
import { useTerminal } from '../hooks/useTerminal'
import { MobileTerminalInput, terminalKeys, type ModifierName, type ModifierState } from '../lib/mobileTerminalInput'

interface TerminalProps {
  sessionName: string
  hostId: string
  fullscreen?: boolean
  onToggleFullscreen?: () => void
}

export function Terminal({ sessionName, hostId, fullscreen, onToggleFullscreen }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<MobileTerminalInput | null>(null)
  if (!inputRef.current) inputRef.current = new MobileTerminalInput()
  const input = inputRef.current
  const [modifiers, setModifiers] = useState<ModifierState>(input.snapshot())
  const [toolbarOpen, setToolbarOpen] = useState(true)
  const [toolbarError, setToolbarError] = useState('')
  const {
    connect, disconnect, fit, focus, termConnected,
    sendInput, copySelection, pasteClipboard,
  } = useTerminal(sessionName, hostId, input)

  useEffect(() => input.subscribe(setModifiers), [input])
  useEffect(() => {
    input.reset()
    setToolbarError('')
  }, [input, sessionName, hostId])

  const cycleModifier = (modifier: ModifierName) => {
    input.cycle(modifier)
    focus()
  }

  const runClipboard = async (action: () => Promise<unknown>) => {
    setToolbarError('')
    try {
      await action()
    } catch (error) {
      setToolbarError(error instanceof Error ? error.message : 'Clipboard operation failed.')
    }
  }

  useEffect(() => {
    if (containerRef.current) {
      connect(containerRef.current)
      requestAnimationFrame(() => focus())
      setTimeout(() => focus(), 100)
    }
    return () => disconnect()
  }, [sessionName, hostId])

  // Refocus terminal when WebSocket reconnects (e.g. after iPad sleep)
  useEffect(() => {
    if (termConnected && !document.hidden) {
      setTimeout(() => {
        fit()
        focus()
        const textarea = containerRef.current?.querySelector('textarea.xterm-helper-textarea') as HTMLTextAreaElement | null
        textarea?.focus()
      }, 100)
    }
  }, [termConnected, fit, focus])

  // Refocus terminal when returning to the app/tab
  useEffect(() => {
    const refocus = () => {
      if (!document.hidden && containerRef.current) {
        setTimeout(() => {
          fit()
          focus()
          // On iPad, xterm's focus() doesn't always work — directly focus the textarea
          const textarea = containerRef.current?.querySelector('textarea.xterm-helper-textarea') as HTMLTextAreaElement | null
          textarea?.focus()
        }, 200)
      }
    }
    document.addEventListener('visibilitychange', refocus)
    // iOS also fires focus on the window when returning from app switcher
    window.addEventListener('focus', refocus)
    return () => {
      document.removeEventListener('visibilitychange', refocus)
      window.removeEventListener('focus', refocus)
    }
  }, [fit, focus])

  useEffect(() => {
    if (!containerRef.current) return
    let frame: number | null = null
    const scheduleFit = () => {
      if (frame !== null) cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        frame = null
        fit()
      })
    }
    const observer = new ResizeObserver(() => {
      scheduleFit()
    })
    observer.observe(containerRef.current)
    window.visualViewport?.addEventListener('resize', scheduleFit)
    window.visualViewport?.addEventListener('scroll', scheduleFit)
    return () => {
      observer.disconnect()
      if (frame !== null) cancelAnimationFrame(frame)
      window.visualViewport?.removeEventListener('resize', scheduleFit)
      window.visualViewport?.removeEventListener('scroll', scheduleFit)
    }
  }, [fit])

  // Touch scroll -> wheel events for tmux mouse mode
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
    const BASE_LINE_HEIGHT = 20
    const MIN_VELOCITY = 0.5
    const FRICTION = 0.92
    const INERTIA_STOP = 0.3

    const dispatchScroll = (lines: number, clientX: number, clientY: number) => {
      const target = container.querySelector('.xterm-screen')
      if (!target) return
      for (let i = 0; i < Math.abs(lines); i++) {
        const wheelEvent = new WheelEvent('wheel', {
          deltaY: lines > 0 ? BASE_LINE_HEIGHT : -BASE_LINE_HEIGHT,
          deltaMode: 0,
          clientX,
          clientY,
          bubbles: true,
          cancelable: true,
        })
        target.dispatchEvent(wheelEvent)
      }
    }

    const processAccumulated = (clientX: number, clientY: number) => {
      const speed = Math.abs(velocity)
      let multiplier = 1
      if (speed > 2) multiplier = 2
      if (speed > 4) multiplier = 3
      if (speed > 7) multiplier = 5
      if (speed > 12) multiplier = 8
      const threshold = BASE_LINE_HEIGHT / multiplier
      while (Math.abs(accumulated) >= threshold) {
        const dir = accumulated > 0 ? 1 : -1
        dispatchScroll(dir, clientX, clientY)
        accumulated -= dir * threshold
      }
    }

    const stopInertia = () => {
      if (inertiaId !== null) {
        cancelAnimationFrame(inertiaId)
        inertiaId = null
      }
    }

    const inertiaLoop = () => {
      if (Math.abs(velocity) < INERTIA_STOP) {
        velocity = 0
        accumulated = 0
        inertiaId = null
        return
      }
      velocity *= FRICTION
      accumulated += velocity * 16
      processAccumulated(lastClientX, lastClientY)
      inertiaId = requestAnimationFrame(inertiaLoop)
    }

    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length !== 1) return
      stopInertia()
      lastY = e.touches[0].clientY
      lastTime = performance.now()
      accumulated = 0
      velocity = 0
    }

    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length !== 1) return
      e.preventDefault()
      const now = performance.now()
      const currentY = e.touches[0].clientY
      const deltaY = lastY - currentY
      const dt = now - lastTime
      lastClientX = e.touches[0].clientX
      lastClientY = e.touches[0].clientY
      if (dt > 0) {
        const instantVelocity = deltaY / dt
        velocity = velocity * 0.3 + instantVelocity * 0.7
      }
      accumulated += deltaY
      lastY = currentY
      lastTime = now
      processAccumulated(lastClientX, lastClientY)
    }

    const onTouchEnd = () => {
      if (Math.abs(velocity) > MIN_VELOCITY) {
        inertiaId = requestAnimationFrame(inertiaLoop)
      }
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

  // Refocus terminal after fullscreen toggle (especially needed on iPad where tapping the button steals focus)
  useEffect(() => {
    setTimeout(() => {
      fit()
      focus()
      const textarea = containerRef.current?.querySelector('textarea.xterm-helper-textarea') as HTMLTextAreaElement | null
      textarea?.focus()
    }, 100)
  }, [fullscreen, fit, focus])

  // Cmd/Ctrl+Shift+F toggles fullscreen, Escape exits (but not if quick switcher is open)
  useEffect(() => {
    if (!onToggleFullscreen) return
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'f') {
        e.preventDefault()
        onToggleFullscreen()
        return
      }
      if (e.key === 'Escape' && fullscreen) {
        // Don't steal Escape from overlays like the quick switcher
        if (document.querySelector('[data-quick-switcher]')) return
        e.preventDefault()
        onToggleFullscreen()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [fullscreen, onToggleFullscreen])

  return (
    <div className="flex-1 p-1 overflow-hidden relative group flex flex-col min-h-0">
      <div
        className="flex-1 min-h-0 w-full border border-border rounded bg-card relative"
        style={{ boxShadow: 'inset 0 0 20px rgba(102, 179, 255, 0.08)' }}
      >
        <div ref={containerRef} className="absolute inset-0.5 overflow-hidden rounded-sm" />
        {/* Fullscreen toggle */}
        {onToggleFullscreen && (
          <button
            onClick={onToggleFullscreen}
            title={fullscreen ? 'Exit fullscreen (Esc / Cmd+Shift+F)' : 'Fullscreen (Cmd+Shift+F)'}
            aria-label={fullscreen ? 'Exit terminal fullscreen' : 'Enter terminal fullscreen'}
            className="absolute top-2 right-2 z-20 min-w-11 min-h-11 md:min-w-0 md:min-h-0 md:p-1.5 rounded bg-card border border-border text-muted-foreground hover:text-primary hover:border-primary/40 transition-colors flex items-center justify-center"
          >
            {fullscreen ? (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="4 14 10 14 10 20" /><polyline points="20 10 14 10 14 4" /><line x1="14" y1="10" x2="21" y2="3" /><line x1="3" y1="21" x2="10" y2="14" />
              </svg>
            ) : (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="15 3 21 3 21 9" /><polyline points="9 21 3 21 3 15" /><line x1="21" y1="3" x2="14" y2="10" /><line x1="3" y1="21" x2="10" y2="14" />
              </svg>
            )}
          </button>
        )}
        {!termConnected && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/85 z-10 pointer-events-none rounded">
            <div className="py-4 px-6 rounded bg-card border border-border text-foreground text-sm font-mono font-bold flex items-center gap-2.5">
              <span className="inline-block w-2 h-2 rounded-full bg-destructive animate-[pulse_1.5s_ease-in-out_infinite]" />
              Disconnected — reconnecting...
            </div>
          </div>
        )}
      </div>
      <div className="md:hidden shrink-0 pt-1 pb-[env(safe-area-inset-bottom)]">
        <button
          type="button"
          className="min-h-11 w-full border border-border rounded bg-card text-xs font-mono"
          aria-expanded={toolbarOpen}
          aria-controls="mobile-terminal-toolbar"
          onClick={() => setToolbarOpen(value => !value)}
        >
          {toolbarOpen ? 'Hide terminal keys' : 'Show terminal keys'}
        </button>
        {toolbarOpen && (
          <div id="mobile-terminal-toolbar" className="mt-1 flex gap-1 overflow-x-auto" role="toolbar" aria-label="Terminal keys">
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
                aria-label={label === '←' ? 'Left arrow' : label === '↑' ? 'Up arrow' : label === '↓' ? 'Down arrow' : label === '→' ? 'Right arrow' : label}
                className="min-w-11 min-h-11 rounded border border-border bg-card font-mono"
                onClick={() => { sendInput(value); focus() }}
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
                className="min-w-14 min-h-11 rounded border border-border bg-card font-mono data-[active=true]:border-primary data-[active=true]:text-primary"
                data-active={modifiers[modifier] !== 'off'}
                onClick={() => cycleModifier(modifier)}
              >
                {modifier === 'ctrl' ? 'Ctrl' : 'Alt'}{modifiers[modifier] === 'locked' ? ' 🔒' : modifiers[modifier] === 'once' ? ' ·' : ''}
              </button>
            ))}
            <button type="button" aria-label="Copy terminal selection" className="min-w-14 min-h-11 rounded border border-border bg-card font-mono" onClick={() => void runClipboard(copySelection)}>Copy</button>
            <button type="button" aria-label="Paste clipboard into terminal" className="min-w-14 min-h-11 rounded border border-border bg-card font-mono" onClick={() => void runClipboard(pasteClipboard)}>Paste</button>
            <button type="button" aria-label="Show software keyboard" className="min-w-14 min-h-11 rounded border border-border bg-card font-mono" onClick={focus}>⌨︎</button>
          </div>
        )}
        {toolbarError && <p className="px-2 pt-1 text-xs text-destructive" role="alert">{toolbarError}</p>}
      </div>
    </div>
  )
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { Terminal, type IDisposable } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import {
  ClipboardAddon,
  type ClipboardSelectionType,
  type IClipboardProvider,
} from '@xterm/addon-clipboard'
import type { ISearchOptions, SearchAddon } from '@xterm/addon-search'
import { usePreferences } from './usePreferences'
import { getXtermTheme } from '../theme'
import type { MobileTerminalInput } from '../lib/mobileTerminalInput'
import {
  TerminalInputError,
  encodeTerminalCommand,
  encodeTerminalPaste,
  isMultilineTerminalPaste,
  terminalTargetKey,
  type TerminalConnectionCapture,
} from '../lib/terminalInput'
import { ensureTerminalFont } from '../fonts'
import '@xterm/xterm/css/xterm.css'

export type PtyConnectionState =
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected'

export interface TerminalSearchState {
  loading: boolean
  loaded: boolean
  error: string
  resultIndex: number
  resultCount: number
}

export interface PendingTerminalPaste extends TerminalConnectionCapture {
  text: string
  multiline: boolean
}

const initialSearchState: TerminalSearchState = {
  loading: false,
  loaded: false,
  error: '',
  resultIndex: -1,
  resultCount: 0,
}

function execCommandCopy(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  let copied = false
  try {
    copied = document.execCommand('copy')
  } catch {
    copied = false
  }
  document.body.removeChild(textarea)
  return copied
}

function clipboardError(action: 'read' | 'write', error?: unknown): Error {
  const message = action === 'read'
    ? 'Clipboard read was denied or is unavailable.'
    : 'Clipboard write was denied or is unavailable.'
  if (error instanceof Error && error.message) return new Error(`${message} ${error.message}`)
  return new Error(message)
}

export function useTerminal(
  sessionName: string,
  hostId: string,
  mobileInput?: MobileTerminalInput,
) {
  const targetKey = terminalTargetKey(hostId, sessionName)
  const termRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const containerRef = useRef<HTMLElement | null>(null)
  const terminalDisposablesRef = useRef<IDisposable[]>([])
  const domCleanupRef = useRef<Array<() => void>>([])
  const layoutCleanupRef = useRef<Array<() => void>>([])
  const reconnectTimerRef = useRef<number | null>(null)
  const reconnectAttemptsRef = useRef(0)
  const terminalGenerationRef = useRef(0)
  const socketGenerationRef = useRef(0)
  const activeTargetKeyRef = useRef('')
  const openSocketRef = useRef<(() => void) | null>(null)
  const pendingClipboardRef = useRef<string | null>(null)
  const searchAddonRef = useRef<SearchAddon | null>(null)
  const searchPromiseRef = useRef<Promise<SearchAddon> | null>(null)
  const searchResultDisposableRef = useRef<IDisposable | null>(null)
  const followOutputRef = useRef(true)
  const { prefs } = usePreferences()

  const [ptyState, setPtyState] = useState<PtyConnectionState>('disconnected')
  const [hasSelection, setHasSelection] = useState(false)
  const [isAtBottom, setIsAtBottom] = useState(true)
  const [hasNewOutput, setHasNewOutput] = useState(false)
  const [searchState, setSearchState] = useState<TerminalSearchState>(initialSearchState)

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current)
      reconnectTimerRef.current = null
    }
  }, [])

  const closeSocket = useCallback(() => {
    const socket = wsRef.current
    wsRef.current = null
    if (!socket) return
    socket.onopen = null
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
  }, [])

  const disposeSearch = useCallback((resetState = true) => {
    searchResultDisposableRef.current?.dispose()
    searchResultDisposableRef.current = null
    searchAddonRef.current?.dispose()
    searchAddonRef.current = null
    searchPromiseRef.current = null
    if (resetState) setSearchState(initialSearchState)
  }, [])

  const cleanupTerminal = useCallback((resetState = true) => {
    clearReconnectTimer()
    openSocketRef.current = null
    closeSocket()
    domCleanupRef.current.splice(0).forEach(cleanup => cleanup())
    layoutCleanupRef.current.splice(0).forEach(cleanup => cleanup())
    terminalDisposablesRef.current.splice(0).forEach(disposable => disposable.dispose())
    disposeSearch(resetState)
    termRef.current?.dispose()
    termRef.current = null
    fitAddonRef.current = null
    containerRef.current = null
    pendingClipboardRef.current = null
    activeTargetKeyRef.current = ''
    terminalGenerationRef.current++
    socketGenerationRef.current++
    followOutputRef.current = true
    reconnectAttemptsRef.current = 0
  }, [clearReconnectTimer, closeSocket, disposeSearch])

  useEffect(() => () => cleanupTerminal(false), [cleanupTerminal])

  const writeClipboard = useCallback(async (text: string, deferOnFailure = false) => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text)
        return
      }
      if (execCommandCopy(text)) return
    } catch (error) {
      if (execCommandCopy(text)) return
      if (!deferOnFailure) throw clipboardError('write', error)
    }
    if (deferOnFailure) {
      pendingClipboardRef.current = text
      return
    }
    throw clipboardError('write')
  }, [])

  const flushPendingClipboard = useCallback(() => {
    const text = pendingClipboardRef.current
    if (text === null) return
    pendingClipboardRef.current = null
    void writeClipboard(text, true)
  }, [writeClipboard])

  const captureConnection = useCallback((): TerminalConnectionCapture | null => {
    const socket = wsRef.current
    if (
      activeTargetKeyRef.current !== targetKey ||
      !socket ||
      socket.readyState !== WebSocket.OPEN
    ) {
      return null
    }
    return {
      targetKey,
      generation: socketGenerationRef.current,
    }
  }, [targetKey])

  const assertCurrentConnection = useCallback((capture: TerminalConnectionCapture) => {
    if (
      capture.targetKey !== targetKey ||
      activeTargetKeyRef.current !== capture.targetKey ||
      socketGenerationRef.current !== capture.generation
    ) {
      throw new TerminalInputError(
        'stale-connection',
        'The Terminal target changed before the input could be sent.',
      )
    }
    const socket = wsRef.current
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      throw new TerminalInputError('not-connected', 'The Terminal is not connected.')
    }
    return socket
  }, [targetKey])

  const sendRawInput = useCallback((
    data: Uint8Array,
    capture?: TerminalConnectionCapture,
  ): boolean => {
    const expected = capture ?? captureConnection()
    if (!expected) {
      throw new TerminalInputError('not-connected', 'The Terminal is not connected.')
    }
    const socket = assertCurrentConnection(expected)
    try {
      socket.send(data)
    } catch {
      throw new TerminalInputError('send-failed', 'The Terminal rejected the input frame.')
    }
    return true
  }, [assertCurrentConnection, captureConnection])

  const sendInput = useCallback((data: string): boolean => {
    try {
      const bytes = mobileInput
        ? mobileInput.encode(data)
        : new TextEncoder().encode(data)
      return sendRawInput(bytes)
    } catch {
      return false
    }
  }, [mobileInput, sendRawInput])

  const sendCommand = useCallback((value: string): boolean => {
    const capture = captureConnection()
    if (!capture) {
      throw new TerminalInputError('not-connected', 'The Terminal is not connected.')
    }
    const frame = encodeTerminalCommand(value)
    sendRawInput(frame, capture)
    mobileInput?.consumeOneShot()
    return true
  }, [captureConnection, mobileInput, sendRawInput])

  const connect = useCallback((container: HTMLElement) => {
    cleanupTerminal()
    if (!hostId) {
      setPtyState('disconnected')
      throw new Error('terminal target is missing host identity')
    }

    const terminalGeneration = ++terminalGenerationRef.current
    activeTargetKeyRef.current = targetKey
    containerRef.current = container
    followOutputRef.current = true
    setHasSelection(false)
    setIsAtBottom(true)
    setHasNewOutput(false)
    setPtyState('connecting')

    const xtermTheme = getXtermTheme(prefs.theme)
    const fontFamily = `'${prefs.terminal.font_family}', 'Symbols Nerd Font Mono', 'JetBrains Mono', Menlo, Monaco, monospace`
    const term = new Terminal({
      theme: xtermTheme,
      fontSize: prefs.terminal.font_size,
      fontFamily,
      cursorBlink: true,
      scrollback: prefs.terminal.scrollback,
      allowProposedApi: true,
      rightClickSelectsWord: true,
      macOptionClickForcesSelection: true,
    })
    const fitAddon = new FitAddon()
    const clipboardProvider: IClipboardProvider = {
      readText(selection: ClipboardSelectionType): Promise<string> {
        if (selection !== 'c') return Promise.resolve('')
        return navigator.clipboard?.readText?.() ?? Promise.resolve('')
      },
      writeText(selection: ClipboardSelectionType, text: string): Promise<void> {
        if (selection !== 'c') return Promise.resolve()
        return writeClipboard(text, true)
      },
    }

    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    term.loadAddon(new ClipboardAddon(undefined, clipboardProvider))
    termRef.current = term
    fitAddonRef.current = fitAddon
    term.open(container)

    const doFit = () => {
      if (
        terminalGenerationRef.current !== terminalGeneration ||
        termRef.current !== term ||
        container.clientWidth <= 0 ||
        container.clientHeight <= 0
      ) {
        return
      }
      try {
        fitAddon.fit()
        if (followOutputRef.current) term.scrollToBottom()
      } catch {
        // Layout can be transiently unavailable while an overlay or viewport changes.
      }
    }

    const animationFrame = window.requestAnimationFrame(doFit)
    layoutCleanupRef.current.push(() => window.cancelAnimationFrame(animationFrame))
    for (const delay of [100, 300]) {
      const timer = window.setTimeout(doFit, delay)
      layoutCleanupRef.current.push(() => window.clearTimeout(timer))
    }

    void ensureTerminalFont(prefs.terminal.font_family).then((loaded) => {
      if (
        !loaded ||
        terminalGenerationRef.current !== terminalGeneration ||
        termRef.current !== term
      ) {
        return
      }
      term.options.fontFamily = fontFamily
      doFit()
    })

    terminalDisposablesRef.current.push(
      term.onSelectionChange(() => setHasSelection(term.hasSelection())),
      term.onScroll(viewportY => {
        const atBottom = viewportY >= term.buffer.active.baseY
        followOutputRef.current = atBottom
        setIsAtBottom(atBottom)
        if (atBottom) setHasNewOutput(false)
      }),
      term.onData(data => {
        const socket = wsRef.current
        if (!socket || socket.readyState !== WebSocket.OPEN) return
        const bytes = mobileInput
          ? mobileInput.encode(data)
          : new TextEncoder().encode(data)
        try {
          socket.send(bytes)
        } catch {
          setPtyState('reconnecting')
        }
      }),
      term.onResize(({ cols, rows }) => {
        const socket = wsRef.current
        if (!socket || socket.readyState !== WebSocket.OPEN) return
        try {
          socket.send(JSON.stringify({ type: 'resize', cols, rows }))
        } catch {
          setPtyState('reconnecting')
        }
      }),
    )

    term.attachCustomKeyEventHandler(event => {
      if (
        event.type === 'keydown' &&
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === 'c' &&
        term.hasSelection()
      ) {
        void writeClipboard(term.getSelection()).then(() => {
          if (terminalGenerationRef.current === terminalGeneration) setHasSelection(false)
        }).catch(() => {})
        term.clearSelection()
        return false
      }
      return true
    })

    container.addEventListener('mousedown', flushPendingClipboard, true)
    container.addEventListener('keydown', flushPendingClipboard, true)
    domCleanupRef.current.push(
      () => container.removeEventListener('mousedown', flushPendingClipboard, true),
      () => container.removeEventListener('keydown', flushPendingClipboard, true),
    )

    const openSocket = () => {
      if (
        terminalGenerationRef.current !== terminalGeneration ||
        termRef.current !== term ||
        document.hidden
      ) {
        return
      }
      clearReconnectTimer()
      closeSocket()

      const generation = ++socketGenerationRef.current
      const cols = term.cols || 80
      const rows = term.rows || 24
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${protocol}//${window.location.host}/ws/session?name=${encodeURIComponent(sessionName)}&cols=${cols}&rows=${rows}&host=${encodeURIComponent(hostId)}`
      const socket = new WebSocket(url)
      socket.binaryType = 'arraybuffer'
      wsRef.current = socket
      setPtyState(reconnectAttemptsRef.current > 0 ? 'reconnecting' : 'connecting')

      const isCurrent = () => (
        terminalGenerationRef.current === terminalGeneration &&
        socketGenerationRef.current === generation &&
        activeTargetKeyRef.current === targetKey &&
        wsRef.current === socket
      )

      socket.onopen = () => {
        if (!isCurrent()) {
          socket.close()
          return
        }
        reconnectAttemptsRef.current = 0
        setPtyState('connected')
      }

      socket.onmessage = event => {
        if (!isCurrent()) return
        if (!followOutputRef.current) setHasNewOutput(true)
        term.write(
          event.data instanceof ArrayBuffer
            ? new Uint8Array(event.data)
            : String(event.data),
        )
      }

      socket.onerror = () => {
        if (isCurrent()) socket.close()
      }

      socket.onclose = () => {
        if (!isCurrent()) return
        wsRef.current = null
        setPtyState('reconnecting')
        if (document.hidden) return
        const attempt = reconnectAttemptsRef.current++
        const delay = Math.min(10_000, 1_000 * 2 ** Math.min(attempt, 4))
        reconnectTimerRef.current = window.setTimeout(() => {
          reconnectTimerRef.current = null
          if (
            terminalGenerationRef.current === terminalGeneration &&
            activeTargetKeyRef.current === targetKey
          ) {
            openSocket()
          }
        }, delay)
      }
    }

    openSocketRef.current = openSocket
    const reconnectWhenVisible = () => {
      if (
        document.hidden ||
        terminalGenerationRef.current !== terminalGeneration ||
        activeTargetKeyRef.current !== targetKey
      ) {
        return
      }
      const socket = wsRef.current
      if (
        !socket ||
        (socket.readyState !== WebSocket.OPEN &&
          socket.readyState !== WebSocket.CONNECTING)
      ) {
        clearReconnectTimer()
        openSocket()
      }
    }
    document.addEventListener('visibilitychange', reconnectWhenVisible)
    window.addEventListener('pageshow', reconnectWhenVisible)
    domCleanupRef.current.push(
      () => document.removeEventListener('visibilitychange', reconnectWhenVisible),
      () => window.removeEventListener('pageshow', reconnectWhenVisible),
    )

    openSocket()
  }, [
    cleanupTerminal,
    clearReconnectTimer,
    closeSocket,
    flushPendingClipboard,
    hostId,
    mobileInput,
    prefs.terminal.font_family,
    prefs.terminal.font_size,
    prefs.terminal.scrollback,
    prefs.theme,
    sessionName,
    targetKey,
    writeClipboard,
  ])

  const disconnect = useCallback(() => {
    cleanupTerminal()
    setPtyState('disconnected')
    setHasSelection(false)
    setIsAtBottom(true)
    setHasNewOutput(false)
  }, [cleanupTerminal])

  const reconnect = useCallback(() => {
    clearReconnectTimer()
    setPtyState('reconnecting')
    openSocketRef.current?.()
  }, [clearReconnectTimer])

  const fit = useCallback(() => {
    const fitAddon = fitAddonRef.current
    const container = containerRef.current
    if (!fitAddon || !container || container.clientWidth <= 0 || container.clientHeight <= 0) {
      return
    }
    try {
      fitAddon.fit()
      if (followOutputRef.current) termRef.current?.scrollToBottom()
    } catch {
      // Ignore transient layout failures while a viewport is resizing.
    }
  }, [])

  const focus = useCallback(() => {
    termRef.current?.focus()
  }, [])

  const scrollToBottom = useCallback(() => {
    followOutputRef.current = true
    termRef.current?.scrollToBottom()
    setIsAtBottom(true)
    setHasNewOutput(false)
    focus()
  }, [focus])

  const adjustFontSize = useCallback((delta: number) => {
    const term = termRef.current
    if (!term) return
    term.options.fontSize = Math.max(8, Math.min(32, (term.options.fontSize || 13) + delta))
    fit()
  }, [fit])

  const copySelection = useCallback(async () => {
    const term = termRef.current
    const terminalGeneration = terminalGenerationRef.current
    const selection = term?.getSelection() ?? ''
    if (!selection) throw new Error('No Terminal text is selected.')
    await writeClipboard(selection)
    if (
      terminalGenerationRef.current !== terminalGeneration ||
      termRef.current !== term
    ) {
      throw new Error('The Terminal target changed after the text was copied.')
    }
    focus()
    return true
  }, [focus, writeClipboard])

  const prepareClipboardPaste = useCallback(async (): Promise<PendingTerminalPaste> => {
    const capture = captureConnection()
    if (!capture) throw new TerminalInputError('not-connected', 'The Terminal is not connected.')
    if (!navigator.clipboard?.readText) throw clipboardError('read')
    let text: string
    try {
      text = await navigator.clipboard.readText()
    } catch (error) {
      throw clipboardError('read', error)
    }
    assertCurrentConnection(capture)
    if (!text) throw new Error('The Clipboard is empty.')
    return {
      ...capture,
      text,
      multiline: isMultilineTerminalPaste(text),
    }
  }, [assertCurrentConnection, captureConnection])

  const commitClipboardPaste = useCallback((paste: PendingTerminalPaste) => {
    assertCurrentConnection(paste)
    const bracketed = termRef.current?.modes.bracketedPasteMode ?? false
    sendRawInput(encodeTerminalPaste(paste.text, bracketed), paste)
    focus()
    return true
  }, [assertCurrentConnection, focus, sendRawInput])

  const pasteClipboard = useCallback(async (): Promise<PendingTerminalPaste | null> => {
    const paste = await prepareClipboardPaste()
    if (paste.multiline) return paste
    commitClipboardPaste(paste)
    return null
  }, [commitClipboardPaste, prepareClipboardPaste])

  const selectAll = useCallback(() => {
    termRef.current?.selectAll()
    setHasSelection(Boolean(termRef.current?.hasSelection()))
  }, [])

  const ensureSearchAddon = useCallback(async (): Promise<SearchAddon> => {
    if (searchAddonRef.current) return searchAddonRef.current
    if (searchPromiseRef.current) return searchPromiseRef.current
    const term = termRef.current
    const terminalGeneration = terminalGenerationRef.current
    if (!term) throw new Error('Open a Terminal before searching.')

    setSearchState(state => ({ ...state, loading: true, error: '' }))
    const promise = import('@xterm/addon-search')
      .then(({ SearchAddon }) => {
        if (
          terminalGenerationRef.current !== terminalGeneration ||
          termRef.current !== term
        ) {
          throw new Error('The Terminal target changed while Search was loading.')
        }
        const addon = new SearchAddon()
        term.loadAddon(addon)
        searchAddonRef.current = addon
        searchResultDisposableRef.current = addon.onDidChangeResults(result => {
          setSearchState(state => ({
            ...state,
            resultIndex: result.resultIndex,
            resultCount: result.resultCount,
          }))
        })
        setSearchState(state => ({ ...state, loading: false, loaded: true, error: '' }))
        return addon
      })
      .catch(error => {
        if (
          terminalGenerationRef.current === terminalGeneration &&
          termRef.current === term
        ) {
          setSearchState(state => ({
            ...state,
            loading: false,
            loaded: false,
            error: error instanceof Error ? error.message : 'Terminal Search failed to load.',
          }))
        }
        throw error
      })
      .finally(() => {
        if (searchPromiseRef.current === promise) searchPromiseRef.current = null
      })
    searchPromiseRef.current = promise
    return promise
  }, [])

  const searchOptions = useCallback((caseSensitive: boolean): ISearchOptions => {
    const theme = getXtermTheme(prefs.theme)
    return {
      caseSensitive,
      incremental: true,
      decorations: {
        matchBackground: theme.brightBlack,
        matchOverviewRuler: theme.blue,
        activeMatchBackground: theme.yellow,
        activeMatchColorOverviewRuler: theme.yellow,
      },
    }
  }, [prefs.theme])

  const findNext = useCallback(async (query: string, caseSensitive = false) => {
    if (!query) {
      searchAddonRef.current?.clearDecorations()
      setSearchState(state => ({ ...state, resultIndex: -1, resultCount: 0 }))
      return false
    }
    const addon = await ensureSearchAddon()
    return addon.findNext(query, searchOptions(caseSensitive))
  }, [ensureSearchAddon, searchOptions])

  const findPrevious = useCallback(async (query: string, caseSensitive = false) => {
    if (!query) return false
    const addon = await ensureSearchAddon()
    return addon.findPrevious(query, searchOptions(caseSensitive))
  }, [ensureSearchAddon, searchOptions])

  const clearSearch = useCallback(() => {
    searchAddonRef.current?.clearDecorations()
    setSearchState(state => ({ ...state, resultIndex: -1, resultCount: 0 }))
  }, [])

  return {
    termRef,
    connect,
    disconnect,
    reconnect,
    fit,
    focus,
    ptyState,
    termConnected: ptyState === 'connected',
    hasSelection,
    isAtBottom,
    hasNewOutput,
    scrollToBottom,
    adjustFontSize,
    sendInput,
    sendRawInput,
    sendCommand,
    captureConnection,
    copySelection,
    prepareClipboardPaste,
    commitClipboardPaste,
    pasteClipboard,
    selectAll,
    searchState,
    ensureSearchAddon,
    findNext,
    findPrevious,
    clearSearch,
  }
}

import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MobileTerminalInput } from '../lib/mobileTerminalInput'
import { terminalTargetKey } from '../lib/terminalInput'
import { useTerminal } from './useTerminal'

const xtermState = vi.hoisted(() => ({
  terminals: [] as any[],
  searchAddons: [] as any[],
  ensureTerminalFont: vi.fn(),
  searchConstructError: null as Error | null,
}))

vi.mock('../fonts', () => ({
  ensureTerminalFont: xtermState.ensureTerminalFont,
}))

vi.mock('./usePreferences', () => ({
  usePreferences: () => ({
    prefs: {
      theme: 'dark',
      terminal: { font_size: 13, font_family: 'JetBrains Mono', scrollback: 5000 },
    },
  }),
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit = vi.fn()
    activate() {}
    dispose() {}
  },
}))

vi.mock('@xterm/addon-web-links', () => ({
  WebLinksAddon: class {
    activate() {}
    dispose() {}
  },
}))

vi.mock('@xterm/addon-clipboard', () => ({
  ClipboardAddon: class {
    activate() {}
    dispose() {}
  },
}))

vi.mock('@xterm/addon-search', () => ({
  SearchAddon: class {
    private listener?: (result: { resultIndex: number; resultCount: number }) => void
    findNext = vi.fn(() => {
      this.listener?.({ resultIndex: 0, resultCount: 2 })
      return true
    })
    findPrevious = vi.fn(() => true)
    clearDecorations = vi.fn()
    dispose = vi.fn()
    onDidChangeResults = (listener: (result: { resultIndex: number; resultCount: number }) => void) => {
      this.listener = listener
      return { dispose: vi.fn() }
    }
    activate() {}
    constructor() {
      if (xtermState.searchConstructError) {
        const error = xtermState.searchConstructError
        xtermState.searchConstructError = null
        throw error
      }
      xtermState.searchAddons.push(this)
    }
  },
}))

vi.mock('@xterm/xterm', () => {
  type Listener<T> = (value: T) => void
  class FakeTerminal {
    cols = 80
    rows = 24
    options: Record<string, any>
    modes = { bracketedPasteMode: false }
    buffer = { active: { baseY: 10, viewportY: 10 } }
    disposed = false
    focused = false
    selection = ''
    scrollBottomCalls = 0
    customKeyHandler?: (event: KeyboardEvent) => boolean
    private selectionListeners = new Set<Listener<void>>()
    private scrollListeners = new Set<Listener<number>>()
    private dataListeners = new Set<Listener<string>>()
    private resizeListeners = new Set<Listener<{ cols: number; rows: number }>>()

    constructor(options: Record<string, any>) {
      this.options = { ...options }
      xtermState.terminals.push(this)
    }

    loadAddon(addon: { activate?: (terminal: FakeTerminal) => void }) {
      addon.activate?.(this)
    }
    open() {}
    dispose() {
      this.disposed = true
      this.selectionListeners.clear()
      this.scrollListeners.clear()
      this.dataListeners.clear()
      this.resizeListeners.clear()
    }
    focus() {
      this.focused = true
    }
    write(_data: unknown) {}
    attachCustomKeyEventHandler(handler: (event: KeyboardEvent) => boolean) {
      this.customKeyHandler = handler
    }
    onSelectionChange(listener: Listener<void>) {
      this.selectionListeners.add(listener)
      return { dispose: () => this.selectionListeners.delete(listener) }
    }
    onScroll(listener: Listener<number>) {
      this.scrollListeners.add(listener)
      return { dispose: () => this.scrollListeners.delete(listener) }
    }
    onData(listener: Listener<string>) {
      this.dataListeners.add(listener)
      return { dispose: () => this.dataListeners.delete(listener) }
    }
    onResize(listener: Listener<{ cols: number; rows: number }>) {
      this.resizeListeners.add(listener)
      return { dispose: () => this.resizeListeners.delete(listener) }
    }
    hasSelection() {
      return Boolean(this.selection)
    }
    getSelection() {
      return this.selection
    }
    clearSelection() {
      this.selection = ''
      this.selectionListeners.forEach(listener => listener())
    }
    selectAll() {
      this.selection = 'all'
      this.selectionListeners.forEach(listener => listener())
    }
    scrollToBottom() {
      this.buffer.active.viewportY = this.buffer.active.baseY
      this.scrollBottomCalls++
      this.scrollListeners.forEach(listener => listener(this.buffer.active.viewportY))
    }
    emitScroll(value: number) {
      this.buffer.active.viewportY = value
      this.scrollListeners.forEach(listener => listener(value))
    }
  }
  return { Terminal: FakeTerminal }
})

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static sockets: FakeWebSocket[] = []
  readonly url: string
  binaryType = ''
  readyState = FakeWebSocket.CONNECTING
  send = vi.fn()
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  constructor(url: string | URL) {
    this.url = String(url)
    FakeWebSocket.sockets.push(this)
  }

  open() {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  message(data: string | ArrayBuffer) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  close() {
    if (this.readyState === FakeWebSocket.CLOSED) return
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }
}

function terminalContainer() {
  const container = document.createElement('div')
  Object.defineProperty(container, 'clientWidth', { configurable: true, value: 800 })
  Object.defineProperty(container, 'clientHeight', { configurable: true, value: 500 })
  return container
}

beforeEach(() => {
  FakeWebSocket.sockets = []
  xtermState.terminals = []
  xtermState.searchAddons = []
  xtermState.ensureTerminalFont.mockResolvedValue(false)
  xtermState.searchConstructError = null
  vi.stubGlobal('WebSocket', FakeWebSocket)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      readText: vi.fn().mockResolvedValue('clipboard'),
      writeText: vi.fn().mockResolvedValue(undefined),
    },
  })
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    callback(0)
    return 1
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
})

describe('useTerminal lifecycle and guarded input', () => {
  it('keeps one xterm and one visibility listener across 100 socket reconnects', () => {
    const addDocument = vi.spyOn(document, 'addEventListener')
    const removeDocument = vi.spyOn(document, 'removeEventListener')
    const { result, unmount } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => result.current.connect(terminalContainer()))
    expect(xtermState.terminals).toHaveLength(1)

    for (let index = 0; index < 100; index++) {
      act(() => {
        FakeWebSocket.sockets[FakeWebSocket.sockets.length - 1].open()
        result.current.reconnect()
      })
    }
    expect(xtermState.terminals).toHaveLength(1)
    expect(addDocument.mock.calls.filter(([type]) => type === 'visibilitychange')).toHaveLength(1)

    unmount()
    expect(xtermState.terminals[0].disposed).toBe(true)
    expect(removeDocument.mock.calls.filter(([type]) => type === 'visibilitychange')).toHaveLength(1)
  })

  it('loads SearchAddon only after Search is requested', async () => {
    const { result } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => result.current.connect(terminalContainer()))
    expect(xtermState.searchAddons).toHaveLength(0)
    await act(async () => {
      await result.current.ensureSearchAddon()
    })
    expect(xtermState.searchAddons).toHaveLength(1)
    await act(async () => {
      await result.current.findNext('hello', true)
    })
    expect(result.current.searchState).toMatchObject({ resultIndex: 0, resultCount: 2 })
  })

  it('surfaces a Search load error, retries, and disposes the addon', async () => {
    xtermState.searchConstructError = new Error('Search chunk unavailable.')
    const { result, unmount } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => result.current.connect(terminalContainer()))

    await act(async () => {
      await expect(result.current.ensureSearchAddon()).rejects.toThrow('Search chunk unavailable.')
    })
    expect(result.current.searchState).toMatchObject({
      loaded: false,
      loading: false,
      error: 'Search chunk unavailable.',
    })

    await act(async () => {
      await result.current.ensureSearchAddon()
    })
    expect(result.current.searchState).toMatchObject({ loaded: true, error: '' })
    const addon = xtermState.searchAddons[0]
    unmount()
    expect(addon.dispose).toHaveBeenCalledTimes(1)
  })

  it('sends one exact command frame and bypasses modifiers', () => {
    const input = new MobileTerminalInput()
    input.cycle('ctrl')
    const { result } = renderHook(() => useTerminal('work', 'host-a', input))
    act(() => {
      result.current.connect(terminalContainer())
      FakeWebSocket.sockets[0].open()
      result.current.sendCommand(' 你好 "$HOME"\nnext ')
    })
    const frames = FakeWebSocket.sockets[0].send.mock.calls
      .map(([frame]) => frame)
      .filter(frame => frame instanceof Uint8Array)
    expect(frames).toHaveLength(1)
    expect(new TextDecoder().decode(frames[0])).toBe(' 你好 "$HOME"\nnext \r')
    expect(input.snapshot().ctrl).toBe('off')

    input.cycle('alt')
    input.cycle('alt')
    act(() => result.current.sendCommand('locked'))
    const lastSend = FakeWebSocket.sockets[0].send.mock.calls[
      FakeWebSocket.sockets[0].send.mock.calls.length - 1
    ][0]
    expect(new TextDecoder().decode(lastSend)).toBe('locked\r')
    expect(input.snapshot().alt).toBe('locked')
  })

  it('rejects stale generations and send exceptions without consuming one-shot modifiers', () => {
    const input = new MobileTerminalInput()
    input.cycle('ctrl')
    const { result } = renderHook(() => useTerminal('work', 'host-a', input))
    act(() => {
      result.current.connect(terminalContainer())
      FakeWebSocket.sockets[0].open()
    })
    const stale = result.current.captureConnection()!
    act(() => {
      result.current.reconnect()
      FakeWebSocket.sockets[1].open()
    })
    expect(() => result.current.sendRawInput(new Uint8Array([1]), stale)).toThrow(/target changed/i)
    expect(FakeWebSocket.sockets[1].send).not.toHaveBeenCalled()

    FakeWebSocket.sockets[1].send.mockImplementationOnce(() => {
      throw new Error('send failed')
    })
    expect(() => result.current.sendCommand('keep draft')).toThrow(/rejected/i)
    expect(input.snapshot().ctrl).toBe('once')
  })

  it('sends zero bytes while closed and keeps one-shot modifiers recoverable', () => {
    const input = new MobileTerminalInput()
    input.cycle('ctrl')
    const { result } = renderHook(() => useTerminal('work', 'host-a', input))
    act(() => result.current.connect(terminalContainer()))

    expect(() => result.current.sendCommand('not yet')).toThrow(/not connected/i)
    expect(FakeWebSocket.sockets[0].send).not.toHaveBeenCalled()
    expect(input.snapshot().ctrl).toBe('once')
  })

  it('copies only the selection and pastes a single-line Clipboard value once', async () => {
    const writeText = vi.mocked(navigator.clipboard.writeText)
    const readText = vi.mocked(navigator.clipboard.readText)
    readText.mockResolvedValueOnce('paste exactly')
    const { result } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => {
      result.current.connect(terminalContainer())
      FakeWebSocket.sockets[0].open()
      xtermState.terminals[0].selection = 'selected only'
    })

    let pasteResult: Awaited<ReturnType<typeof result.current.pasteClipboard>>
    await act(async () => {
      await result.current.copySelection()
      pasteResult = await result.current.pasteClipboard()
    })
    expect(writeText).toHaveBeenCalledWith('selected only')
    expect(pasteResult!).toBeNull()
    expect(FakeWebSocket.sockets[0].send).toHaveBeenCalledTimes(1)
    const frame = FakeWebSocket.sockets[0].send.mock.calls[0][0] as Uint8Array
    expect(new TextDecoder().decode(frame)).toBe('paste exactly')
  })

  it('sends zero Clipboard bytes when the socket generation changes during read', async () => {
    let resolveRead!: (value: string) => void
    const readPromise = new Promise<string>(resolve => {
      resolveRead = resolve
    })
    vi.mocked(navigator.clipboard.readText).mockReturnValueOnce(readPromise)
    const { result } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => {
      result.current.connect(terminalContainer())
      FakeWebSocket.sockets[0].open()
    })

    const paste = result.current.pasteClipboard()
    const rejection = expect(paste).rejects.toThrow(/target changed/i)
    act(() => {
      result.current.reconnect()
      FakeWebSocket.sockets[1].open()
      resolveRead('must not leak')
    })
    await rejection
    expect(FakeWebSocket.sockets[0].send).not.toHaveBeenCalled()
    expect(FakeWebSocket.sockets[1].send).not.toHaveBeenCalled()
  })

  it('latches scrollback while output arrives and resumes at the bottom', () => {
    const { result } = renderHook(() => useTerminal('work', 'host-a'))
    act(() => {
      result.current.connect(terminalContainer())
      FakeWebSocket.sockets[0].open()
      xtermState.terminals[0].emitScroll(3)
      FakeWebSocket.sockets[0].message('new output')
    })
    expect(result.current.isAtBottom).toBe(false)
    expect(result.current.hasNewOutput).toBe(true)

    act(() => result.current.scrollToBottom())
    expect(result.current.isAtBottom).toBe(true)
    expect(result.current.hasNewOutput).toBe(false)
  })

  it('isolates captures for same-name sessions on different hosts', () => {
    const { result, rerender } = renderHook(
      ({ host }) => useTerminal('work', host),
      { initialProps: { host: 'host-a' } },
    )
    act(() => {
      result.current.connect(terminalContainer())
      FakeWebSocket.sockets[0].open()
    })
    const capture = result.current.captureConnection()!
    expect(capture.targetKey).toBe(terminalTargetKey('host-a', 'work'))
    rerender({ host: 'host-b' })
    expect(() => result.current.sendRawInput(new Uint8Array([1]), capture)).toThrow(/target changed/i)
  })
})

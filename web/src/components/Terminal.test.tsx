import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Terminal } from './Terminal'

const terminal = {
  connect: vi.fn(),
  disconnect: vi.fn(),
  fit: vi.fn(),
  focus: vi.fn(),
  termConnected: true,
  sendInput: vi.fn(() => true),
  copySelection: vi.fn<() => Promise<void>>(),
  pasteClipboard: vi.fn<() => Promise<boolean>>(),
}

vi.mock('../hooks/useTerminal', () => ({
  useTerminal: () => terminal,
}))

describe('mobile Terminal controls', () => {
  afterEach(cleanup)
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
    vi.clearAllMocks()
    terminal.copySelection.mockResolvedValue()
    terminal.pasteClipboard.mockResolvedValue(true)
  })

  it('sends special keys and resets locked modifiers after target change', () => {
    const view = render(<Terminal sessionName="one" hostId="host-a" />)
    fireEvent.click(screen.getByRole('button', { name: 'Up arrow' }))
    expect(terminal.sendInput).toHaveBeenCalledWith('\x1b[A')

    fireEvent.click(screen.getByRole('button', { name: /Control modifier: off/ }))
    fireEvent.click(screen.getByRole('button', { name: /Control modifier: once/ }))
    expect(screen.getByRole('button', { name: /Control modifier: locked/ })).toHaveAttribute('aria-pressed', 'true')

    view.rerender(<Terminal sessionName="two" hostId="host-b" />)
    expect(screen.getByRole('button', { name: /Control modifier: off/ })).toHaveAttribute('aria-pressed', 'false')
  })

  it('reports clipboard permission failures without sending stale data', async () => {
    terminal.pasteClipboard.mockRejectedValueOnce(new Error('Clipboard permission denied.'))
    fireEvent.click(render(<Terminal sessionName="one" hostId="host-a" />).getByRole('button', { name: 'Paste clipboard into terminal' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Clipboard permission denied.'))
  })

  it('exposes 44px touch controls and a collapsible toolbar', () => {
    render(<Terminal sessionName="one" hostId="host-a" />)
    const toggle = screen.getByRole('button', { name: 'Hide terminal keys' })
    expect(toggle.className).toContain('min-h-11')
    fireEvent.click(toggle)
    expect(screen.queryByRole('toolbar', { name: 'Terminal keys' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show terminal keys' })).toHaveAttribute('aria-expanded', 'false')
  })
})

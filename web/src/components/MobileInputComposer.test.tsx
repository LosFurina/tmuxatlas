import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MobileInputComposer } from './MobileInputComposer'

function renderComposer(onSend = vi.fn<() => void | Promise<void>>()) {
  const onDraftChange = vi.fn()
  render(
    <MobileInputComposer
      targetKey='["host-a","work"]'
      targetLabel="host-a/work"
      initialDraft=""
      onDraftChange={onDraftChange}
      onSend={onSend}
    />,
  )
  fireEvent.click(screen.getByRole('button', { name: 'Expand Mobile Input Composer' }))
  return { onSend, onDraftChange }
}

describe('MobileInputComposer boundaries', () => {
  afterEach(cleanup)

  beforeEach(() => {
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      callback(0)
      return 1
    })
  })

  it('allows an empty submission as a physical Enter equivalent', async () => {
    const { onSend } = renderComposer()
    fireEvent.click(screen.getByRole('button', { name: 'Send command to host-a/work' }))
    await waitFor(() => expect(onSend).toHaveBeenCalledWith(''))
  })

  it('accepts 65,535 UTF-8 body bytes', async () => {
    const { onSend } = renderComposer()
    const value = 'x'.repeat(65_535)
    fireEvent.change(screen.getByRole('textbox'), { target: { value } })
    fireEvent.click(screen.getByRole('button', { name: 'Send command to host-a/work' }))
    await waitFor(() => expect(onSend).toHaveBeenCalledWith(value))
  })

  it('rejects 65,536 UTF-8 body bytes and preserves the draft', () => {
    const { onSend } = renderComposer()
    const value = 'x'.repeat(65_536)
    const textarea = screen.getByRole('textbox')
    fireEvent.change(textarea, { target: { value } })
    expect(screen.getByRole('alert')).toHaveTextContent('65,535')
    expect(screen.getByRole('button', { name: 'Send command to host-a/work' })).toBeDisabled()
    expect(textarea).toHaveValue(value)
    expect(onSend).not.toHaveBeenCalled()
  })

  it('focuses the textarea when expanded and preserves a failed draft', async () => {
    const onSend = vi.fn().mockRejectedValue(new Error('Socket closed.'))
    renderComposer(onSend)
    const textarea = screen.getByRole('textbox')
    expect(textarea).toHaveFocus()
    fireEvent.change(textarea, { target: { value: 'keep this' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send command to host-a/work' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Socket closed.')
    expect(textarea).toHaveValue('keep this')
  })

  it('does not clear text entered while an earlier draft is sending', async () => {
    let resolveSend!: () => void
    const onSend = vi.fn(() => new Promise<void>(resolve => {
      resolveSend = resolve
    }))
    renderComposer(onSend)
    const textarea = screen.getByRole('textbox')
    fireEvent.change(textarea, { target: { value: 'first' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send command to host-a/work' }))
    expect(onSend).toHaveBeenCalledWith('first')

    fireEvent.change(textarea, { target: { value: 'typed while sending' } })
    resolveSend()

    await waitFor(() => expect(textarea).toHaveValue('typed while sending'))
  })

  it('flushes the current native value when navigation blurs the composer', () => {
    const { onDraftChange } = renderComposer()
    const textarea = screen.getByRole('textbox')
    fireEvent.change(textarea, { target: { value: 'draft before navigation' } })
    onDraftChange.mockClear()

    fireEvent.blur(textarea)

    expect(onDraftChange).toHaveBeenCalledWith(
      '["host-a","work"]',
      'draft before navigation',
    )
  })
})

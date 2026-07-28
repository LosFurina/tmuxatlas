import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Setup } from './Setup'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Setup', () => {
  it('renders the Hub setup state when the legacy endpoint returns an empty object', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })))

    render(<Setup onComplete={vi.fn()} />)

    expect(await screen.findByText('This Hub does not require local agent setup. Install and pair an Agent on each tmux host.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeInTheDocument()
  })
})

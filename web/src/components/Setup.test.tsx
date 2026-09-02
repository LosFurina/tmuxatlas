import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { normalizeAgentStatus, Setup } from './Setup'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Setup', () => {
	  it('normalizes passive Agy without making setup blocking', () => {
	    expect(normalizeAgentStatus({ agents: [{ name: 'Agy', key: 'agy', installed: true, configured: false, tracking_mode: 'passive', setup_required: true }] })).toMatchObject({ agents: [{ tracking_mode: 'passive', setup_required: false }] })
	  })

	  it('uses hook defaults for malformed optional metadata', () => {
	    expect(normalizeAgentStatus({ agents: [{ name: 'Agent', key: 'agent', installed: true, configured: false, tracking_mode: 'invalid', setup_required: false }] })).toMatchObject({ agents: [{ tracking_mode: 'hook', setup_required: true }] })
	  })

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

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  PreferencesContext,
  defaultPreferences,
  type Preferences,
} from '../hooks/usePreferences'
import type { PWAInstallState } from '../hooks/usePWAInstall'
import { Settings } from './Settings'

type SaveState = 'idle' | 'saving' | 'saved' | 'error'

const pwaInstall: PWAInstallState = {
  canPrompt: false,
  isStandalone: false,
  isAppleMobile: false,
  outcome: 'idle',
  install: vi.fn(),
}

function contextValue({
  prefs = defaultPreferences,
  retrySave = vi.fn().mockResolvedValue(undefined),
  saveError = null,
  saveState = 'idle',
}: {
  prefs?: Preferences
  retrySave?: ReturnType<typeof vi.fn>
  saveError?: string | null
  saveState?: SaveState
} = {}) {
  return {
    prefs,
    updatePrefs: vi.fn().mockResolvedValue(undefined),
    loaded: true,
    refetch: vi.fn().mockResolvedValue(undefined),
    saveState,
    saveError,
    retrySave,
  }
}

function settings(value: ReturnType<typeof contextValue>) {
  return (
    <PreferencesContext.Provider value={value}>
      <Settings
        pushState="unsupported"
        onPushSubscribe={vi.fn()}
        onPushUnsubscribe={vi.fn()}
        pwaInstall={pwaInstall}
      />
    </PreferencesContext.Provider>
  )
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 503 })))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Settings preference save feedback', () => {
  it('renders Hub agent settings when the legacy endpoint returns an empty object', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })))

    render(settings(contextValue()))

    expect(await screen.findByText('This Hub does not require local agent setup. Install and pair an Agent on each tmux host.')).toBeInTheDocument()
  })

  it('announces saving and saved states and shows a success toast', async () => {
    const { rerender } = render(settings(contextValue()))

    rerender(settings(contextValue({ saveState: 'saving' })))
    expect(screen.getByText('Saving…')).toHaveAttribute('role', 'status')

    rerender(settings(contextValue({ saveState: 'saved' })))
    expect(screen.getByText('Saved')).toHaveAttribute('role', 'status')
    expect(await screen.findByText('Settings saved')).toBeInTheDocument()
    expect(screen.getByText('Your preferences are up to date.')).toBeInTheDocument()
  })

  it('keeps an explicit unsaved warning and retries the failed payload', async () => {
    const retrySave = vi.fn().mockResolvedValue(undefined)
    const { rerender } = render(settings(contextValue()))

    rerender(settings(contextValue({
      retrySave,
      saveError: 'Failed to save preferences (503).',
      saveState: 'error',
    })))

    expect(screen.getByText('Changes were not saved.')).toBeInTheDocument()
    expect(screen.getAllByText(/Failed to save preferences \(503\)\./)).toHaveLength(2)
    expect(await screen.findByText('Settings not saved')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry saving preferences' }))
    await waitFor(() => expect(retrySave).toHaveBeenCalledTimes(1))

    rerender(settings(contextValue({ retrySave, saveState: 'saving' })))
    expect(screen.queryByText('Changes were not saved.')).not.toBeInTheDocument()
    expect(screen.queryByText('Settings not saved')).not.toBeInTheDocument()
    expect(screen.getByText('Saving…')).toBeInTheDocument()
  })
})

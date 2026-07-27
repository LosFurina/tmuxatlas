import { useState } from 'react'
import { readFileSync } from 'node:fs'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { applyTheme, semanticThemeVars, themePresets } from '../../theme'
import {
  Button,
  Dialog,
  EmptyState,
  IconButton,
  Kbd,
  Menu,
  MenuItem,
  Sheet,
  Skeleton,
  StatusPill,
  ToastProvider,
  Tooltip,
  useToast,
} from '.'

const indexCss = readFileSync('src/index.css', 'utf8')

afterEach(() => {
  cleanup()
  document.documentElement.removeAttribute('style')
})

describe('semantic theme contracts', () => {
  it('provides every semantic token in every existing preset', () => {
    for (const preset of Object.values(themePresets)) {
      for (const [token, value] of Object.entries(semanticThemeVars)) {
        expect(preset.cssVars[token], `${preset.name} is missing ${token}`).toBe(value)
      }
    }
  })

  it('applies semantic aliases while preserving existing themes', () => {
    applyTheme('light')
    expect(document.documentElement.style.getPropertyValue('--surface-raised')).toBe('var(--card)')
    expect(document.documentElement.style.getPropertyValue('--focus-ring')).toBe('var(--ring)')
    expect(document.documentElement.style.getPropertyValue('--terminal-chrome')).toBe('var(--card)')

    applyTheme('retro-blue')
    expect(document.documentElement.style.getPropertyValue('--surface-raised')).toBe('var(--card)')
    expect(document.documentElement.style.getPropertyValue('--status-waiting')).toBe('var(--warning)')
  })

  it('uses a readable UI stack and reserves monospace for technical content', () => {
    expect(indexCss).toContain('--font-sans: ui-sans-serif')
    expect(indexCss).toContain("--font-mono: 'JetBrains Mono'")
    expect(indexCss).toContain("--font-display: 'VT323'")
  })

  it('scopes touch capture to xterm and disables decorative motion when requested', () => {
    const rootRule = indexCss.match(/html, body, #root \{[\s\S]*?\n  \}/)?.[0] ?? ''
    expect(rootRule).not.toContain('touch-action: none')
    expect(indexCss).toMatch(/\.xterm,[\s\S]*?touch-action: none;/)
    expect(indexCss).toContain('@media (prefers-reduced-motion: reduce)')
    expect(indexCss).toContain('animation-duration: 0.01ms !important')
  })

  it('guarantees a 44 by 44 CSS pixel coarse-pointer target', () => {
    expect(semanticThemeVars['--control-target-min']).toBe('44px')
    expect(indexCss).toContain('@media (pointer: coarse), (max-width: 767px)')
    expect(indexCss).toContain('min-inline-size: max(var(--control-target-min), 44px)')
    expect(indexCss).toContain('min-block-size: max(var(--control-target-min), 44px)')
  })
})

describe('basic shared controls', () => {
  it('supports keyboard activation, loading semantics and a unified focus class', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Reconnect</Button>)

    await user.tab()
    const button = screen.getByRole('button', { name: 'Reconnect' })
    expect(button).toHaveFocus()
    expect(button).toHaveClass('ui-focus-ring')
    await user.keyboard('{Enter}')
    expect(onClick).toHaveBeenCalledTimes(1)

    cleanup()
    render(<Button loading loadingLabel="Connecting">Reconnect</Button>)
    expect(screen.getByRole('button', { name: 'Connecting' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Connecting' })).toHaveAttribute('aria-busy', 'true')
  })

  it('requires an accessible name for icon-only controls', () => {
    render(<IconButton aria-label="Open terminal actions">•••</IconButton>)
    const button = screen.getByRole('button', { name: 'Open terminal actions' })
    expect(button).toHaveClass('ui-icon-button', 'ui-focus-ring')
  })

  it('connects a keyboard tooltip to its control', async () => {
    const user = userEvent.setup()
    render(
      <Tooltip content="Search terminal">
        <button type="button">Find</button>
      </Tooltip>,
    )

    await user.tab()
    const tooltip = screen.getByRole('tooltip')
    expect(tooltip).toHaveTextContent('Search terminal')
    expect(screen.getByRole('button', { name: 'Find' })).toHaveAttribute('aria-describedby', tooltip.id)
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  it('expresses status with a visible label and exposes semantic kbd markup', () => {
    render(
      <>
        <StatusPill status="waiting">Waiting for input</StatusPill>
        <Kbd>⌘K</Kbd>
      </>,
    )
    expect(screen.getByText('Waiting for input').closest('[data-status]')).toHaveAttribute('data-status', 'waiting')
    expect(screen.getByText('⌘K').tagName).toBe('KBD')
  })
})

describe('accessible overlays', () => {
  function DialogHarness() {
    const [open, setOpen] = useState(false)
    return (
      <>
        <button type="button" onClick={() => setOpen(true)}>Open preferences</button>
        <Dialog
          open={open}
          onOpenChange={setOpen}
          title="Preferences"
          description="Workspace preferences"
          footer={<button type="button">Save</button>}
        >
          <button type="button">Reset</button>
        </Dialog>
      </>
    )
  }

  it('traps focus, makes the background inert, closes on Escape and restores focus', async () => {
    const user = userEvent.setup()
    const view = render(<DialogHarness />)
    const trigger = screen.getByRole('button', { name: 'Open preferences' })
    await user.click(trigger)

    const dialog = await screen.findByRole('dialog', { name: 'Preferences' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(view.container).toHaveAttribute('aria-hidden', 'true')
    expect(view.container.inert).toBe(true)
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement))

    const save = screen.getByRole('button', { name: 'Save' })
    save.focus()
    await user.tab()
    expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await waitFor(() => expect(trigger).toHaveFocus())
    expect(view.container).not.toHaveAttribute('aria-hidden')
    expect(view.container.inert).toBe(false)
  })

  it('uses the same dialog semantics for a bottom sheet', () => {
    render(
      <Sheet open onOpenChange={() => {}} title="Commands" side="bottom">
        Sheet content
      </Sheet>,
    )
    expect(screen.getByRole('dialog', { name: 'Commands' })).toHaveClass('ui-sheet')
    expect(screen.getByRole('dialog', { name: 'Commands' })).toHaveAttribute('data-side', 'bottom')
  })

  it('navigates menu items with arrows and restores the trigger after selection', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(
      <Menu label="Terminal actions" trigger={<span aria-hidden="true">•••</span>}>
        <MenuItem>Copy</MenuItem>
        <MenuItem onSelect={onSelect}>Find</MenuItem>
      </Menu>,
    )
    const trigger = screen.getByRole('button', { name: 'Terminal actions' })
    await user.click(trigger)
    const items = await screen.findAllByRole('menuitem')
    await waitFor(() => expect(items[0]).toHaveFocus())
    await user.keyboard('{ArrowDown}{Enter}')
    expect(onSelect).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    await waitFor(() => expect(trigger).toHaveFocus())
  })
})

describe('feedback and page states', () => {
  function ToastHarness() {
    const { toast } = useToast()
    return (
      <button
        type="button"
        onClick={() => toast({ title: 'Connected', description: 'PTY is ready', duration: 0 })}
      >
        Notify
      </button>
    )
  }

  it('announces and dismisses toast feedback', async () => {
    const user = userEvent.setup()
    render(
      <ToastProvider>
        <ToastHarness />
      </ToastProvider>,
    )
    await user.click(screen.getByRole('button', { name: 'Notify' }))
    expect(screen.getByRole('status')).toHaveTextContent('Connected')
    expect(screen.getByRole('status')).toHaveTextContent('PTY is ready')
    await user.click(screen.getByRole('button', { name: 'Dismiss notification' }))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('keeps decorative skeletons hidden and gives empty states an action', () => {
    render(
      <>
        <Skeleton data-testid="skeleton" />
        <EmptyState
          title="No sessions"
          description="Create a session to get started."
          action={<Button>New Session</Button>}
        />
      </>,
    )
    expect(screen.getByTestId('skeleton')).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByRole('heading', { name: 'No sessions' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'New Session' })).toBeInTheDocument()
  })
})

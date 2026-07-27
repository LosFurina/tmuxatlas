import { expect, test, type Page } from '@playwright/test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const baseURL = 'http://localhost:17658'
const fixedNow = Date.parse('2026-07-27T03:00:00Z')
let server: ChildProcessWithoutNullStreams
let testHome: string

const preferences = (theme: 'light' | 'dark' | 'retro-blue') => ({
  terminal: { font_size: 13, font_family: 'JetBrains Mono', scrollback: 5000 },
  theme,
  custom_theme: {},
  sidebar: { default_collapsed: false, hidden_sessions: [], collapse_mode: 'small' },
  default_view: 'overview',
  notifications: { statuses: ['waiting', 'error', 'completed'] },
  agent_banner: { auto_dismiss_seconds: 0 },
  quick_switcher_shortcut: 'ctrl+k',
  sparklines_visible: true,
  overview_refresh_interval: 5,
  timestamp_format: 'relative',
  lock_timeout_minutes: 30,
  lock_background_faster: true,
  lock_background_minutes: 10,
  fullscreen_hide_alerts: true,
})

const snapshot = {
  type: 'snapshot',
  schema_version: 1,
  instance_id: 'visual-workspace',
  revision: 1,
  state: {
    hosts: {
      'host/host-a': {
        key: 'host/host-a',
        id: 'host-a',
        display_name: 'studio-mac',
        online: true,
        local: true,
      },
      'host/host-b': {
        key: 'host/host-b',
        id: 'host-b',
        display_name: 'build-node',
        online: true,
        local: false,
      },
    },
    sessions: {
      'session/host-a/workspace': {
        key: 'session/host-a/workspace',
        host_key: 'host/host-a',
        host_id: 'host-a',
        name: 'workspace',
        attached: true,
        last_activity: '2026-07-27T02:59:30Z',
      },
      'session/host-b/deploy': {
        key: 'session/host-b/deploy',
        host_key: 'host/host-b',
        host_id: 'host-b',
        name: 'deploy',
        attached: false,
        last_activity: '2026-07-27T02:57:00Z',
      },
    },
    windows: {
      'window/host-a/workspace/0': {
        key: 'window/host-a/workspace/0',
        session_key: 'session/host-a/workspace',
        tmux_id: '@1',
        name: 'codex',
        index: 0,
        active: true,
      },
      'window/host-b/deploy/0': {
        key: 'window/host-b/deploy/0',
        session_key: 'session/host-b/deploy',
        tmux_id: '@2',
        name: 'release',
        index: 0,
        active: true,
      },
    },
    panes: {
      'pane/host-a/workspace/0/%1': {
        key: 'pane/host-a/workspace/0/%1',
        window_key: 'window/host-a/workspace/0',
        tmux_id: '%1',
        index: 0,
        active: true,
        width: 120,
        height: 34,
        current_command: 'codex',
      },
    },
    tool_events: {
      'tool/host-b/deploy/codex': {
        key: 'tool/host-b/deploy/codex',
        host_id: 'host-b',
        session: 'deploy',
        window: '0',
        pane: '%2',
        tool: 'codex',
        status: 'waiting',
        message: 'Approval required',
        timestamp: '2026-07-27T02:58:30Z',
      },
    },
    activity: {
      'activity/host-a/workspace': {
        key: 'activity/host-a/workspace',
        host_id: 'host-a',
        session: 'workspace',
        timestamp: '2026-07-27T02:59:30Z',
        data: { idle_seconds: 0, sparkline: [1, 2, 3, 2, 4, 5, 3, 5] },
      },
    },
    health: {},
    metadata: { server: { version: 'v0.visual' } },
  },
}

async function waitForServer() {
  await expect.poll(async () => {
    try {
      return (await fetch(`${baseURL}/api/version`)).ok
    } catch {
      return false
    }
  }).toBe(true)
}

async function openWorkspace(
  page: Page,
  theme: 'light' | 'dark' | 'retro-blue',
  viewport: { width: number; height: number },
  expandComposer: boolean,
) {
  await page.setViewportSize(viewport)
  await page.addInitScript(now => {
    Date.now = () => now
  }, fixedNow)
  await page.route('**/api/preferences', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(preferences(theme)),
  }))
  await page.routeWebSocket('**/ws/events?schema=1', socket => {
    socket.send(JSON.stringify(snapshot))
  })
  await page.routeWebSocket('**/ws/session**', socket => {
    socket.send([
      '\u001b[1;36mTmuxAtlas Workspace\u001b[0m',
      '',
      '$ codex --workspace tmuxatlas',
      'Analyzing terminal UX and connection safety…',
      '✓ Command registry ready',
      '✓ PTY generation isolated',
      '',
      '$ _',
    ].join('\r\n'))
  })
  await page.goto(`${baseURL}/session/host-a/workspace`)
  await expect(page.getByText('Connected', { exact: true })).toBeVisible()
  if (expandComposer) {
    await page.getByRole('button', { name: 'Expand Mobile Input Composer' }).click()
    await page.getByRole('textbox', { name: /studio-mac\/workspace/ }).fill('git status\nnpm test')
  }
  await page.addStyleTag({
    content: `
      *, *::before, *::after {
        animation-duration: 0s !important;
        transition-duration: 0s !important;
        caret-color: transparent !important;
      }
      .xterm-cursor-layer { visibility: hidden !important; }
    `,
  })
  await page.evaluate(() => document.fonts.ready)
}

test.beforeAll(async () => {
  testHome = await mkdtemp(join(tmpdir(), 'tmuxatlas-visual-e2e-'))
  server = spawn(resolve(process.cwd(), '..', 'dist', 'tmuxatlas'), [
    'server',
    '--listen', '127.0.0.1:17658',
    '--public-url', baseURL,
    '--discovery-interval', '60',
    '--no-auth',
  ], { env: { ...process.env, HOME: testHome } })
  await waitForServer()
})

test.afterAll(async () => {
  server?.kill('SIGTERM')
  await rm(testHome, { recursive: true, force: true })
})

test('Light Workspace at 1440×900', async ({ page }) => {
  await openWorkspace(page, 'light', { width: 1440, height: 900 }, false)
  await expect(page).toHaveScreenshot('workspace-light-1440x900.png', {
    animations: 'disabled',
    caret: 'hide',
  })
})

test('Dark Workspace at 390×844', async ({ page }) => {
  await openWorkspace(page, 'dark', { width: 390, height: 844 }, true)
  await expect(page).toHaveScreenshot('workspace-dark-390x844.png', {
    animations: 'disabled',
    caret: 'hide',
  })
})

test('Retro Workspace at 844×390', async ({ page }) => {
  await openWorkspace(page, 'retro-blue', { width: 844, height: 390 }, true)
  await expect(page).toHaveScreenshot('workspace-retro-844x390.png', {
    animations: 'disabled',
    caret: 'hide',
  })
})

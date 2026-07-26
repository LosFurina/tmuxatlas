import { test, expect } from '@playwright/test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const baseURL = 'http://localhost:17655'
let server: ChildProcessWithoutNullStreams
let testHome: string

test.use({
  channel: process.env.CI ? undefined : 'chrome',
})

async function waitForServer() {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const response = await fetch(`${baseURL}/api/version`)
      if (response.ok) return
    } catch {
      // Server is still starting.
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 100))
  }
  throw new Error('TmuxAtlas PWA E2E server did not become ready')
}

test.beforeAll(async () => {
  testHome = await mkdtemp(join(tmpdir(), 'tmuxatlas-pwa-e2e-'))
  const binary = resolve(process.cwd(), '..', 'dist', 'tmuxatlas')
  server = spawn(binary, [
    'server',
    '--listen', '127.0.0.1:17655',
    '--public-url', baseURL,
    '--discovery-interval', '60',
    '--no-auth',
  ], {
    env: { ...process.env, HOME: testHome },
  })
  await waitForServer()
})

test.afterAll(async () => {
  server?.kill('SIGTERM')
  await rm(testHome, { recursive: true, force: true })
})

test('serves install metadata and a network-neutral root Service Worker', async ({ page, request }) => {
  const manifestResponse = await request.get(`${baseURL}/manifest.json`)
  expect(manifestResponse.ok()).toBe(true)
  expect(manifestResponse.headers()['content-type']).toContain('application/manifest+json')
  const manifest = await manifestResponse.json()
  expect(manifest).toMatchObject({
    id: '/',
    name: 'TmuxAtlas',
    short_name: 'TmuxAtlas',
    start_url: '/',
    scope: '/',
    display: 'standalone',
    background_color: '#0d1117',
    theme_color: '#0d1117',
  })
  expect(manifest.icons.some((icon: { purpose?: string }) => icon.purpose === 'any')).toBe(true)
  expect(manifest.icons.some((icon: { purpose?: string }) => icon.purpose === 'maskable')).toBe(true)
  for (const icon of manifest.icons as { src: string; sizes: string; type: string }[]) {
    const iconResponse = await request.get(`${baseURL}${icon.src}`)
    expect(iconResponse.ok(), `${icon.src} should be available`).toBe(true)
    expect(iconResponse.headers()['content-type']).toContain(icon.type)
  }

  const workerResponse = await request.get(`${baseURL}/sw.js`)
  expect(workerResponse.ok()).toBe(true)
  expect(workerResponse.headers()['content-type']).toContain('text/javascript')
  expect(workerResponse.headers()['cache-control']).toBe('no-cache')
  expect(workerResponse.headers()['service-worker-allowed']).toBe('/')
  const workerSource = await workerResponse.text()
  expect(workerSource).not.toMatch(/addEventListener\(['"]fetch/)
  expect(workerSource).not.toContain('caches.')

  await page.goto(baseURL)
  const registration = await page.evaluate(async () => {
    const registered = await navigator.serviceWorker.register('/sw.js', {
      scope: '/',
      updateViaCache: 'none',
    })
    const ready = await navigator.serviceWorker.ready
    return {
      scriptURL: (ready.active || registered.active)?.scriptURL,
      scope: ready.scope,
      updateViaCache: ready.updateViaCache,
    }
  })
  expect(registration.scriptURL).toBe(`${baseURL}/sw.js`)
  expect(registration.scope).toBe(`${baseURL}/`)
  expect(registration.updateViaCache).toBe('none')

  await page.reload()
  await expect.poll(() => page.evaluate(() => navigator.serviceWorker.controller?.scriptURL)).toBe(`${baseURL}/sw.js`)
  expect(await page.evaluate(() => caches.keys())).toEqual([])

  let revision = 0
  await page.route('**/api/version?e2e-pwa=1', async (route) => {
    revision += 1
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ revision }),
    })
  })
  const revisions = await page.evaluate(async () => {
    const first = await fetch('/api/version?e2e-pwa=1').then((response) => response.json())
    const second = await fetch('/api/version?e2e-pwa=1').then((response) => response.json())
    return [first.revision, second.revision]
  })
  expect(revisions).toEqual([1, 2])
})

test('uses an explicit browser install gesture and suppresses repeat installation', async ({ page }) => {
  await page.goto(baseURL)

  const dispatchInstallPrompt = async (outcome: 'accepted' | 'dismissed') => {
    await page.evaluate((choice) => {
      const installWindow = window as typeof window & { __installPromptCalls?: number }
      installWindow.__installPromptCalls = 0
      const event = new Event('beforeinstallprompt', { cancelable: true })
      Object.defineProperties(event, {
        prompt: {
          value: async () => {
            installWindow.__installPromptCalls = (installWindow.__installPromptCalls || 0) + 1
          },
        },
        userChoice: {
          value: Promise.resolve({ outcome: choice, platform: 'web' }),
        },
      })
      window.dispatchEvent(event)
    }, outcome)
  }

  await dispatchInstallPrompt('dismissed')
  await page.getByTitle('Settings').click()
  await page.getByRole('button', { name: 'Install TmuxAtlas' }).click()
  await expect(page.getByText('Installation dismissed.')).toBeVisible()
  expect(await page.evaluate(() => (
    window as typeof window & { __installPromptCalls?: number }
  ).__installPromptCalls)).toBe(1)

  await dispatchInstallPrompt('accepted')
  await page.getByRole('button', { name: 'Install TmuxAtlas' }).click()
  await expect(page.getByText('Installation accepted.')).toBeVisible()

  await page.evaluate(() => window.dispatchEvent(new Event('appinstalled')))
  await expect(page.getByTestId('pwa-installed')).toHaveText('Installed')
  await expect(page.getByRole('button', { name: 'Install TmuxAtlas' })).toHaveCount(0)
})

test('shows manual Add to Home Screen guidance on Apple mobile browsers', async ({ browser }) => {
  const context = await browser.newContext({
    userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1',
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
  })
  const page = await context.newPage()
  await page.goto(baseURL)
  await page.getByTitle('Settings').click()

  await expect(page.getByTestId('pwa-ios-guidance')).toContainText('tap Share')
  await expect(page.getByTestId('pwa-ios-guidance')).toContainText('Add to Home Screen')
  await expect(page.getByRole('button', { name: 'Install TmuxAtlas' })).toHaveCount(0)
  await context.close()
})

test('keeps Push payloads and notification clicks on the Hub origin', async ({ page }) => {
  await page.goto(baseURL)
  const result = await page.evaluate(async () => {
    const source = await fetch('/sw.js').then((response) => response.text())
    const handlers: Record<string, (event: any) => void> = {}
    const notifications: { title: string; options: NotificationOptions }[] = []
    const navigations: string[] = []
    let focused = 0
    let opened = ''
    const client = {
      url: `${location.origin}/`,
      navigate: async (url: string) => {
        navigations.push(url)
        return client
      },
      focus: async () => {
        focused += 1
        return client
      },
    }
    const worker = {
      location: { origin: location.origin },
      addEventListener: (type: string, handler: (event: any) => void) => {
        handlers[type] = handler
      },
      skipWaiting: async () => undefined,
      registration: {
        showNotification: async (title: string, options: NotificationOptions) => {
          notifications.push({ title, options })
        },
      },
      clients: {
        claim: async () => undefined,
        matchAll: async () => [client],
        openWindow: async (url: string) => {
          opened = url
          return client
        },
      },
    }

    new Function('self', source)(worker)

    let pushCompletion: Promise<unknown> = Promise.resolve()
    handlers.push({
      data: {
        json: () => ({
          title: 'Codex needs input',
          body: 'waiting in session "project.v2"',
          host_id: 'host-a',
          session: 'project.v2',
        }),
      },
      waitUntil: (completion: Promise<unknown>) => {
        pushCompletion = completion
      },
    })
    await pushCompletion

    let malformedCompletion: Promise<unknown> = Promise.resolve()
    handlers.push({
      data: {
        json: () => { throw new Error('invalid') },
        text: () => 'not-json',
      },
      waitUntil: (completion: Promise<unknown>) => {
        malformedCompletion = completion
      },
    })
    await malformedCompletion

    let clickCompletion: Promise<unknown> = Promise.resolve()
    handlers.notificationclick({
      notification: {
        data: { url: 'https://attacker.example/session/stolen' },
        close: () => undefined,
      },
      waitUntil: (completion: Promise<unknown>) => {
        clickCompletion = completion
      },
    })
    await clickCompletion

    return {
      notifications,
      navigations,
      focused,
      opened,
    }
  })

  expect(result.notifications[0]).toMatchObject({
    title: 'Codex needs input',
    options: {
      body: 'waiting in session "project.v2"',
      data: { url: '/session/host-a/project.v2' },
    },
  })
  expect(result.notifications[1]).toMatchObject({
    title: 'TmuxAtlas',
    options: {
      data: { url: '/' },
    },
  })
  expect(result.navigations).toEqual(['/'])
  expect(result.focused).toBe(1)
  expect(result.opened).toBe('')
})

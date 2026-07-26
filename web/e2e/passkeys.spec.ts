import {
  test,
  expect,
  chromium,
  type Browser,
  type BrowserContext,
  type CDPSession,
  type Page,
} from '@playwright/test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const baseURL = 'http://localhost:17654'
let server: ChildProcessWithoutNullStreams
let testHome: string
let setupToken: string
let browser: Browser
let context: BrowserContext
let page: Page
let cdp: CDPSession
let authenticatorId: string

async function waitForServer() {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const response = await fetch(`${baseURL}/api/auth/status`)
      if (response.ok) return
    } catch {
      // Server is still starting.
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 100))
  }
  throw new Error('TmuxAtlas E2E server did not become ready')
}

test.beforeAll(async () => {
  testHome = await mkdtemp(join(tmpdir(), 'tmuxatlas-e2e-'))
  const binary = resolve(process.cwd(), '..', 'dist', 'tmuxatlas')
  server = spawn(binary, [
    'server',
    '--listen', '127.0.0.1:17654',
    '--public-url', baseURL,
    '--discovery-interval', '60',
  ], {
    env: { ...process.env, HOME: testHome, TMUXATLAS_SESSION_TTL: '1h' },
  })
  let logs = ''
  const collect = (chunk: Buffer) => {
    logs += chunk.toString()
    const match = logs.match(/"setup_token":"([A-Za-z0-9_-]+)"/)
    if (match) setupToken = match[1]
  }
  server.stdout.on('data', collect)
  server.stderr.on('data', collect)
  await waitForServer()
  for (let attempt = 0; attempt < 50 && !setupToken; attempt++) {
    await new Promise((resolveWait) => setTimeout(resolveWait, 100))
  }
  if (!setupToken) throw new Error(`Setup token was not found in server logs:\n${logs}`)

  browser = await chromium.launch({ channel: process.env.CI ? 'chromium' : 'chrome' })
  context = await browser.newContext()
  page = await context.newPage()
  cdp = await context.newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  const authenticator = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  })
  authenticatorId = authenticator.authenticatorId
})

test.afterAll(async () => {
  await context?.close()
  await browser?.close()
  server?.kill('SIGTERM')
  await rm(testHome, { recursive: true, force: true })
})

test('complete Passkey lifecycle', async () => {
  await page.goto(baseURL)
  await page.getByPlaceholder('One-time setup token from server logs').fill(setupToken)
  await page.getByPlaceholder('Passkey label (optional)').fill('Primary')
  await page.getByRole('button', { name: 'Create passkey' }).click()

  await expect(page.getByRole('heading', { name: 'agent setup' })).toBeVisible()
  await page.getByRole('button', { name: 'Next' }).click()
  await page.getByRole('button', { name: 'Continue to Dashboard' }).click()

  await page.getByTitle('Settings').click()
  await expect(page.getByText('Primary', { exact: true })).toBeVisible()
  const registeredCredentials = await cdp.send('WebAuthn.getCredentials', { authenticatorId })
  expect(registeredCredentials.credentials).toHaveLength(1)
  expect(registeredCredentials.credentials[0].isResidentCredential).toBe(true)
  await page.getByRole('button', { name: 'Sign out' }).click()
  await cdp.send('WebAuthn.setUserVerified', { authenticatorId, isUserVerified: true })
  await cdp.send('WebAuthn.setAutomaticPresenceSimulation', {
    authenticatorId,
    enabled: true,
  })
  await page.getByRole('button', { name: 'Sign in with passkey' }).click()
  await page.getByTitle('Settings').click()

  await cdp.send('WebAuthn.setAutomaticPresenceSimulation', {
    authenticatorId,
    enabled: false,
  })
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'usb',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  })
  await page.getByLabel('New passkey label').fill('Backup')
  await page.getByRole('button', { name: 'Add passkey' }).click()
  await expect(page.getByTestId('passkey-row')).toHaveCount(2)

  const backup = page.getByTestId('passkey-row').filter({ hasText: 'Backup' })
  await backup.getByRole('button', { name: 'Rename' }).click()
  await page.getByLabel('Passkey label', { exact: true }).fill('Travel key')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Travel key', { exact: true })).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  const primary = page.getByTestId('passkey-row').filter({ hasText: 'Primary' })
  await primary.getByRole('button', { name: 'Delete' }).click()
  await expect(page.getByTestId('passkey-row')).toHaveCount(1)
  await expect(page.getByRole('button', { name: 'Delete' })).toBeDisabled()

  const finalDeleteStatus = await page.evaluate(async () => {
    const inventory = await fetch('/api/auth/passkeys').then((response) => response.json())
    return fetch(`/api/auth/passkeys/${encodeURIComponent(inventory.passkeys[0].id)}`, {
      method: 'DELETE',
    }).then((response) => response.status)
  })
  expect(finalDeleteStatus).toBe(409)
  await expect(page.getByText('Travel key', { exact: true })).toBeVisible()
})

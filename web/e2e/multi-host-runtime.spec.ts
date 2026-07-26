import { test, expect } from '@playwright/test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const baseURL = 'http://localhost:17656'
let server: ChildProcessWithoutNullStreams
let testHome: string

test.use({
  channel: process.env.CI ? undefined : 'chrome',
})

const localHost = { id: 'local-host', name: 'local', local: true, online: true, sessions: [], last_seen: new Date().toISOString() }
const remoteHost = { id: 'remote-host', name: 'remote', local: false, online: true, sessions: [], last_seen: new Date().toISOString() }
const session = (host: string, hostName: string) => ({
  id: `${host}-same`, name: 'same', host, host_name: hostName, host_online: true,
  windows: [
    { id: `${host}-w0`, session_id: `${host}-same`, name: 'shell', index: 0, active: true, layout: '', panes: [] },
    { id: `${host}-w1`, session_id: `${host}-same`, name: 'editor', index: 1, active: false, layout: '', panes: [] },
  ],
  created: new Date().toISOString(), attached: false, last_activity: new Date().toISOString(),
})

async function waitForServer() {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      if ((await fetch(`${baseURL}/api/version`)).ok) return
    } catch {}
    await new Promise(resolveWait => setTimeout(resolveWait, 100))
  }
  throw new Error('TmuxAtlas multi-host E2E server did not become ready')
}

test.beforeAll(async () => {
  testHome = await mkdtemp(join(tmpdir(), 'tmuxatlas-multi-host-e2e-'))
  const binary = resolve(process.cwd(), '..', 'dist', 'tmuxatlas')
  server = spawn(binary, [
    'server', '--listen', '127.0.0.1:17656', '--public-url', baseURL,
    '--discovery-interval', '60', '--no-auth',
  ], { env: { ...process.env, HOME: testHome } })
  await waitForServer()
})

test.afterAll(async () => {
  server?.kill('SIGTERM')
  await rm(testHome, { recursive: true, force: true })
})

test('keeps same-name sessions bound to explicit hosts and surfaces structured errors', async ({ page }) => {
  const mutations: { path: string; body: any }[] = []
  const terminalFrames: (string | Buffer)[] = []
  const websocketURLs: string[] = []
  let rejectRename = false
  let remoteSessionName = 'same'
  await page.routeWebSocket('**/ws/session**', socket => {
    websocketURLs.push(socket.url())
    socket.onMessage(message => terminalFrames.push(message))
  })
  await page.route('**/api/sessions', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify([
      session(localHost.id, localHost.name),
      { ...session(remoteHost.id, remoteHost.name), name: remoteSessionName },
    ]),
  }))
  await page.route('**/api/hosts', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify([localHost, remoteHost]),
  }))
  await page.route('**/api/session/**', async route => {
    const request = route.request()
    const body = request.postDataJSON()
    mutations.push({ path: new URL(request.url()).pathname, body })
    if (rejectRename && request.url().endsWith('/rename')) {
      await route.fulfill({
        status: 503, contentType: 'application/json',
        body: JSON.stringify({ request_id: 'failed-request', code: 'peer-offline' }),
      })
      return
    }
    if (request.url().endsWith('/rename') && body.host_id === 'remote-host') {
      remoteSessionName = body.new_name
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ request_id: 'ok', result: { ok: true } }),
    })
  })

  await page.goto(baseURL)

  const remoteGroup = page.getByRole('listitem').filter({ hasText: 'remotesame' })
  const remoteSession = remoteGroup.getByRole('button').filter({ hasText: 'same' })
  await remoteSession.click()
  await expect(page).toHaveURL(`${baseURL}/session/remote-host/same`)
  await expect.poll(() => websocketURLs.some(url =>
    url.includes('/ws/session?') && url.includes('host=remote-host') && url.includes('name=same'),
  )).toBe(true)
  await page.setViewportSize({ width: 980, height: 720 })
  await page.locator('.xterm-helper-textarea').pressSequentially('typed')
  await expect.poll(() => terminalFrames.some(frame =>
    typeof frame === 'string' && JSON.parse(frame).type === 'resize',
  )).toBe(true)
  await expect.poll(() => Buffer.concat(
    terminalFrames.filter((frame): frame is Buffer => Buffer.isBuffer(frame)),
  ).toString().includes('typed')).toBe(true)

  await page.keyboard.press('Control+k')
  await page.getByPlaceholder('Go to session or window...').fill('remoteeditor')
  await page.getByText('remote: same/editor', { exact: true }).click()
  await expect.poll(() => mutations.find(entry => entry.path.endsWith('/select-window'))?.body).toEqual({
    host_id: 'remote-host', session: 'same', window: 1,
  })

  await remoteSession.click({ button: 'right' })
  await page.getByText('Rename', { exact: true }).click()
  const renameInput = page.locator('aside input').first()
  await renameInput.fill('renamed')
  await renameInput.press('Enter')
  await expect.poll(() => mutations.find(entry => entry.path.endsWith('/rename'))?.body).toEqual({
    host_id: 'remote-host', session: 'same', new_name: 'renamed',
  })
  await expect(page).toHaveURL(`${baseURL}/session/remote-host/renamed`)

  await page.getByTitle('New session').click()
  await page.getByPlaceholder('Session name...').fill('created')
  await page.locator('select').selectOption('remote-host')
  await page.getByRole('button', { name: 'Create' }).click()
  await expect.poll(() => mutations.find(entry => entry.path.endsWith('/new'))?.body).toEqual({
    host_id: 'remote-host', session: 'created',
  })

  rejectRename = true
  await page.goto(baseURL)
  const localGroup = page.getByRole('listitem').filter({ hasText: 'localsame' })
  const localSession = localGroup.getByRole('button').filter({ hasText: 'same' })
  await localSession.click({ button: 'right' })
  await page.getByText('Rename', { exact: true }).click()
  const failedRenameInput = page.locator('aside input').first()
  await failedRenameInput.fill('will-fail')
  await failedRenameInput.press('Enter')
  await expect(page.getByText('The selected host is offline.')).toBeVisible()
  const failed = mutations.filter(entry => entry.path.endsWith('/rename')).at(-1)
  expect(failed?.body).toEqual({
    host_id: 'local-host', session: 'same', new_name: 'will-fail',
  })
})

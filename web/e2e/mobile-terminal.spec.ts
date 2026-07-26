import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const baseURL = 'http://localhost:17657'
let server: ChildProcessWithoutNullStreams
let testHome: string

test.beforeAll(async () => {
  testHome = await mkdtemp(join(tmpdir(), 'tmuxatlas-mobile-e2e-'))
  server = spawn(resolve(process.cwd(), '..', 'dist', 'tmuxatlas'), [
    'server', '--listen', '127.0.0.1:17657', '--public-url', baseURL,
    '--discovery-interval', '60', '--no-auth',
  ], { env: { ...process.env, HOME: testHome } })
  await expect.poll(async () => {
    try { return (await fetch(`${baseURL}/api/version`)).ok } catch { return false }
  }).toBe(true)
})

test.afterAll(async () => {
  server?.kill('SIGTERM')
  await rm(testHome, { recursive: true, force: true })
})

test('mobile drawer, terminal toolbar, orientation and accessibility', async ({ page }) => {
  await page.routeWebSocket('**/ws/events?schema=1', socket => {
    socket.send(JSON.stringify({
      type: 'snapshot', schema_version: 1, instance_id: 'mobile-e2e', revision: 1,
      state: {
        hosts: { 'host/host-a': { key: 'host/host-a', id: 'host-a', display_name: 'phone-agent', online: true, local: false } },
        sessions: { 'session/host-a/demo': { key: 'session/host-a/demo', host_key: 'host/host-a', host_id: 'host-a', name: 'demo', attached: false } },
        windows: {}, panes: {}, tool_events: {}, activity: {}, health: {}, metadata: { server: { version: 'e2e' } },
      },
    }))
  })
  await page.routeWebSocket('**/ws/session**', () => {})
  await page.goto(baseURL)

  await page.getByRole('button', { name: 'Toggle session drawer' }).click()
  await expect(page.getByRole('complementary', { name: 'Sessions' })).toHaveClass(/translate-x-0/)
  await page.getByRole('button', { name: /demo/ }).click()
  await expect(page.getByRole('toolbar', { name: 'Terminal keys' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Up arrow' })).toHaveClass(/min-h-11/)

  await page.setViewportSize({ width: 844, height: 390 })
  await expect(page.getByRole('button', { name: 'Hide terminal keys' })).toBeVisible()
  const results = await new AxeBuilder({ page }).analyze()
  expect(results.violations.filter(violation => violation.impact === 'critical')).toEqual([])
})

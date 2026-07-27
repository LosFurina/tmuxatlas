import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Locator, type Page } from '@playwright/test'
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { Buffer } from 'node:buffer'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import {
  capturePTY,
  type PTYCapture,
  type PTYTarget,
} from './ptyCapture'

const baseURL = 'http://localhost:17657'
let server: ChildProcessWithoutNullStreams
let testHome: string

interface SnapshotHost {
  id: string
  name: string
  local: boolean
}

interface SnapshotSession {
  hostId: string
  name: string
}

interface MockVisualViewportValue {
  width: number
  height: number
  offsetTop: number
  offsetLeft: number
}

interface SafeAreaInsets {
  top: number
  right: number
  bottom: number
  left: number
}

const zeroSafeArea: SafeAreaInsets = {
  top: 0,
  right: 0,
  bottom: 0,
  left: 0,
}

const singleHostWorkspace = {
  hosts: [{ id: 'host-a', name: 'phone-agent', local: false }],
  sessions: [{ hostId: 'host-a', name: 'demo' }],
}

const sameNameWorkspace = {
  hosts: [
    { id: 'host-a', name: 'alpha', local: true },
    { id: 'host-b', name: 'beta', local: false },
  ],
  sessions: [
    { hostId: 'host-a', name: 'work' },
    { hostId: 'host-b', name: 'work' },
  ],
}

test.beforeAll(async () => {
  testHome = await mkdtemp(join(tmpdir(), 'tmuxatlas-mobile-e2e-'))
  server = spawn(resolve(process.cwd(), '..', 'dist', 'tmuxatlas'), [
    'server', '--listen', '127.0.0.1:17657', '--public-url', baseURL,
    '--discovery-interval', '60', '--no-auth',
  ], { env: { ...process.env, HOME: testHome } })
  await expect.poll(async () => {
    try {
      return (await fetch(`${baseURL}/api/version`)).ok
    } catch {
      return false
    }
  }).toBe(true)
})

test.afterAll(async () => {
  server?.kill('SIGTERM')
  await rm(testHome, { recursive: true, force: true })
})

async function mockWorkspace(
  page: Page,
  workspace: {
    hosts: SnapshotHost[]
    sessions: SnapshotSession[]
  },
) {
  await page.routeWebSocket('**/ws/events?schema=1', socket => {
    const hosts = Object.fromEntries(workspace.hosts.map(host => [
      `host/${host.id}`,
      {
        key: `host/${host.id}`,
        id: host.id,
        display_name: host.name,
        online: true,
        local: host.local,
      },
    ]))
    const sessions = Object.fromEntries(workspace.sessions.map(session => [
      `session/${session.hostId}/${session.name}`,
      {
        key: `session/${session.hostId}/${session.name}`,
        host_key: `host/${session.hostId}`,
        host_id: session.hostId,
        name: session.name,
        attached: false,
      },
    ]))
    const windows = Object.fromEntries(workspace.sessions.map((session, index) => [
      `window/${session.hostId}/${session.name}/0`,
      {
        key: `window/${session.hostId}/${session.name}/0`,
        session_key: `session/${session.hostId}/${session.name}`,
        tmux_id: `@${index + 1}`,
        name: 'shell',
        index: 0,
        active: true,
      },
    ]))
    socket.send(JSON.stringify({
      type: 'snapshot',
      schema_version: 1,
      instance_id: 'mobile-e2e',
      revision: 1,
      state: {
        hosts,
        sessions,
        windows,
        panes: {},
        tool_events: {},
        activity: {},
        health: {},
        metadata: { server: { version: 'e2e' } },
      },
    }))
  })
}

async function openSession(
  page: Page,
  hostName: string,
  hostId: string,
  sessionName: string,
) {
  const viewport = page.viewportSize()
  let navigation: Locator
  if ((viewport?.width ?? 0) < 1024) {
    await page.getByRole('button', { name: 'Toggle session drawer' }).click()
    const drawer = page.getByRole('dialog', { name: 'Workspace sessions' })
    await expect(drawer).toBeVisible()
    navigation = drawer
  } else {
    navigation = page.getByRole('navigation', { name: 'Host and session navigation' })
  }
  await navigation.getByRole('button', {
    name: new RegExp(`^Open ${hostName} session ${sessionName},`, 'i'),
  }).click()
  await expect(page).toHaveURL(
    `${baseURL}/session/${encodeURIComponent(hostId)}/${encodeURIComponent(sessionName)}`,
  )
}

async function expandComposer(page: Page): Promise<Locator> {
  const expand = page.getByRole('button', { name: 'Expand Mobile Input Composer' })
  if (await expand.isVisible()) await expand.click()
  const textarea = page.getByRole('textbox', { name: /^Command draft for / })
  await expect(textarea).toBeVisible()
  return textarea
}

async function sendComposerValue({
  page,
  capture,
  target,
  generation,
  textarea,
  value,
}: {
  page: Page
  capture: PTYCapture
  target: PTYTarget
  generation: number
  textarea: Locator
  value: string
}) {
  await textarea.fill(value)
  const mark = capture.mark()
  await page.getByRole('button', { name: /^Send command to / }).click()
  const frames = await capture.waitForInputFrames({
    target,
    generation,
    afterOrdinal: mark,
    count: 1,
  })
  await page.waitForTimeout(25)
  const capturedAfterMark = capture.inputFrames(target, generation)
    .filter(frame => frame.ordinal > mark)
  expect(capturedAfterMark, `Composer must emit one frame for ${JSON.stringify(value)}`).toHaveLength(1)
  expect(frames[0]).toMatchObject({
    hostId: target.hostId,
    sessionName: target.sessionName,
    generation,
  })
  expect(frames[0].bytes).toEqual(Buffer.from(`${value}\r`, 'utf8'))
  await expect(textarea).toHaveValue('')
}

async function armGenerationRace(page: Page, value: string) {
  await page.evaluate(raceValue => {
    const testWindow = window as typeof window & {
      __raceComposerValueForE2E?: string
    }
    testWindow.__raceComposerValueForE2E = raceValue
    const nativeSend = WebSocket.prototype.send
    WebSocket.prototype.send = function sendWithGenerationRace(data) {
      const isBinary = data instanceof ArrayBuffer || ArrayBuffer.isView(data)
      if (
        testWindow.__raceComposerValueForE2E
        && this.url.includes('/ws/session')
        && isBinary
      ) {
        testWindow.__raceComposerValueForE2E = undefined
        throw new DOMException('E2E generation changed before PTY write', 'InvalidStateError')
      }
      return nativeSend.call(this, data)
    }
  }, value)
}

async function installMockVisualViewport(page: Page) {
  await page.addInitScript(() => {
    const state = {
      width: window.innerWidth,
      height: window.innerHeight,
      offsetTop: 0,
      offsetLeft: 0,
    }
    const events = new EventTarget()
    const viewport = {
      get width() { return state.width },
      get height() { return state.height },
      get offsetTop() { return state.offsetTop },
      get offsetLeft() { return state.offsetLeft },
      get pageTop() { return state.offsetTop },
      get pageLeft() { return state.offsetLeft },
      get scale() { return 1 },
      addEventListener: events.addEventListener.bind(events),
      removeEventListener: events.removeEventListener.bind(events),
      dispatchEvent: events.dispatchEvent.bind(events),
    }
    Object.defineProperty(window, 'visualViewport', {
      configurable: true,
      get: () => viewport,
    })
    Object.defineProperty(window, '__setVisualViewportForE2E', {
      configurable: true,
      value: (next: Partial<typeof state>) => {
        Object.assign(state, next)
        events.dispatchEvent(new Event('resize'))
        events.dispatchEvent(new Event('scroll'))
      },
    })
  })
}

async function setMockVisualViewport(page: Page, value: MockVisualViewportValue) {
  await page.evaluate(next => {
    const testWindow = window as typeof window & {
      __setVisualViewportForE2E: (viewport: MockVisualViewportValue) => void
    }
    testWindow.__setVisualViewportForE2E(next)
  }, value)
  await expect.poll(() => page.evaluate(() => (
    getComputedStyle(document.documentElement).getPropertyValue('--visual-viewport-height').trim()
  ))).toBe(`${Math.round(value.height)}px`)
}

async function setSafeAreaInsets(page: Page, value: SafeAreaInsets) {
  await page.evaluate(insets => {
    const root = document.documentElement
    root.style.setProperty('--safe-area-inset-top', `${insets.top}px`)
    root.style.setProperty('--safe-area-inset-right', `${insets.right}px`)
    root.style.setProperty('--safe-area-inset-bottom', `${insets.bottom}px`)
    root.style.setProperty('--safe-area-inset-left', `${insets.left}px`)
  }, value)
  await expect.poll(() => page.evaluate(() => {
    const style = getComputedStyle(document.documentElement)
    return [
      style.getPropertyValue('--safe-area-inset-top').trim(),
      style.getPropertyValue('--safe-area-inset-right').trim(),
      style.getPropertyValue('--safe-area-inset-bottom').trim(),
      style.getPropertyValue('--safe-area-inset-left').trim(),
    ]
  })).toEqual([
    `${value.top}px`,
    `${value.right}px`,
    `${value.bottom}px`,
    `${value.left}px`,
  ])
}

async function expectInsideVisualViewport(
  locator: Locator,
  label: string,
  safeArea: SafeAreaInsets = zeroSafeArea,
  safeEdges: Partial<Record<'top' | 'right' | 'bottom' | 'left', boolean>> = {},
) {
  await expect(locator, `${label} should be visible`).toBeVisible()
  await expect.poll(async () => locator.evaluate((element, options) => {
    const rect = element.getBoundingClientRect()
    const viewport = window.visualViewport
    const viewportLeft = viewport?.offsetLeft ?? 0
    const viewportTop = viewport?.offsetTop ?? 0
    const viewportRight = viewportLeft + (viewport?.width ?? window.innerWidth)
    const viewportBottom = viewportTop + (viewport?.height ?? window.innerHeight)
    const left = viewportLeft + (options.safeEdges.left ? options.safeArea.left : 0)
    const top = viewportTop + (options.safeEdges.top ? options.safeArea.top : 0)
    const right = viewportRight - (options.safeEdges.right ? options.safeArea.right : 0)
    const bottom = viewportBottom - (options.safeEdges.bottom ? options.safeArea.bottom : 0)
    return (
      rect.left >= left - 1
      && rect.top >= top - 1
      && rect.right <= right + 1
      && rect.bottom <= bottom + 1
    )
  }, { safeArea, safeEdges }), `${label} should stay inside the visual viewport`).toBe(true)
}

async function expectNoShellOverflow(page: Page) {
  const layout = await page.evaluate(() => {
    const shell = document.querySelector<HTMLElement>('.app-shell')
    if (!shell) throw new Error('Missing app shell')
    const rect = shell.getBoundingClientRect()
    const viewport = window.visualViewport
    return {
      left: rect.left,
      top: rect.top,
      right: rect.right,
      bottom: rect.bottom,
      scrollWidth: shell.scrollWidth,
      clientWidth: shell.clientWidth,
      viewportLeft: viewport?.offsetLeft ?? 0,
      viewportTop: viewport?.offsetTop ?? 0,
      viewportRight: (viewport?.offsetLeft ?? 0) + (viewport?.width ?? window.innerWidth),
      viewportBottom: (viewport?.offsetTop ?? 0) + (viewport?.height ?? window.innerHeight),
    }
  })
  expect(layout.left).toBeGreaterThanOrEqual(layout.viewportLeft - 1)
  expect(layout.top).toBeGreaterThanOrEqual(layout.viewportTop - 1)
  expect(layout.right).toBeLessThanOrEqual(layout.viewportRight + 1)
  expect(layout.bottom).toBeLessThanOrEqual(layout.viewportBottom + 1)
  expect(layout.scrollWidth).toBeLessThanOrEqual(layout.clientWidth + 1)
}

test('mobile WebKit Composer sends each complete value as exactly one Binary UTF-8 frame', async ({ page }) => {
  await mockWorkspace(page, singleHostWorkspace)
  const pty = await capturePTY(page)
  await page.goto(baseURL)
  await openSession(page, 'phone-agent', 'host-a', 'demo')

  const target = { hostId: 'host-a', sessionName: 'demo' }
  const connection = await pty.waitForConnection(target)
  const textarea = await expandComposer(page)
  const values = [
    '中文🙂',
    '  printf "$HOME" && echo done  ',
    'first line\n第二行\nthird line',
    '',
  ]
  for (const value of values) {
    await sendComposerValue({
      page,
      capture: pty,
      target,
      generation: connection.generation,
      textarea,
      value,
    })
  }

  expect(pty.inputFrames(target, connection.generation)).toHaveLength(values.length)
})

test('same-name cross-Host drafts and connection generations never leak PTY input', async ({ page }) => {
  await mockWorkspace(page, sameNameWorkspace)
  const pty = await capturePTY(page)
  await page.goto(baseURL)

  const targetA = { hostId: 'host-a', sessionName: 'work' }
  const targetB = { hostId: 'host-b', sessionName: 'work' }
  await openSession(page, 'alpha', targetA.hostId, targetA.sessionName)
  const firstA = await pty.waitForConnection(targetA)
  let textarea = await expandComposer(page)
  await textarea.fill('draft for alpha')

  await openSession(page, 'beta', targetB.hostId, targetB.sessionName)
  const firstB = await pty.waitForConnection(targetB)
  textarea = await expandComposer(page)
  await expect(textarea).toHaveValue('')
  await textarea.fill('draft for beta')

  await openSession(page, 'alpha', targetA.hostId, targetA.sessionName)
  const currentA = await pty.waitForConnection(targetA, {
    afterGeneration: firstA.generation,
  })
  textarea = await expandComposer(page)
  await expect(textarea).toHaveValue('draft for alpha')
  expect(pty.inputFrames()).toHaveLength(0)
  await sendComposerValue({
    page,
    capture: pty,
    target: targetA,
    generation: currentA.generation,
    textarea,
    value: 'draft for alpha',
  })
  expect(pty.inputFrames(targetA, firstA.generation)).toHaveLength(0)
  expect(pty.inputFrames(targetB, firstB.generation)).toHaveLength(0)

  await openSession(page, 'beta', targetB.hostId, targetB.sessionName)
  const currentB = await pty.waitForConnection(targetB, {
    afterGeneration: firstB.generation,
  })
  textarea = await expandComposer(page)
  await expect(textarea).toHaveValue('draft for beta')

  const raceValue = 'must remain on beta after generation race'
  await textarea.fill(raceValue)
  await armGenerationRace(page, raceValue)
  const mark = pty.mark()
  await page.getByRole('button', { name: /^Send command to / }).click()
  await expect(page.getByRole('alert')).toContainText('Terminal rejected the input frame.')
  await expect(textarea).toHaveValue(raceValue)

  await currentB.socket.close({
    code: 1012,
    reason: 'E2E generation rollover after rejected write',
  })
  const nextB = await pty.waitForConnection(targetB, {
    afterGeneration: currentB.generation,
  })
  expect(nextB.generation).toBeGreaterThan(currentB.generation)
  await page.waitForTimeout(100)
  expect(pty.inputFrames().filter(frame => frame.ordinal > mark)).toHaveLength(0)
  expect(pty.inputFrames(targetA, currentA.generation)).toHaveLength(1)
  expect(pty.inputFrames(targetB, currentB.generation)).toHaveLength(0)
  expect(pty.inputFrames(targetB, nextB.generation)).toHaveLength(0)
  await expect(textarea).toHaveValue(raceValue)
})

test('Drawer, Palette, Terminal toolbar and Composer stay within mobile visual viewports', async ({ page }) => {
  test.setTimeout(90_000)
  await installMockVisualViewport(page)
  await mockWorkspace(page, singleHostWorkspace)
  await capturePTY(page)
  await page.goto(baseURL)
  await openSession(page, 'phone-agent', 'host-a', 'demo')

  const sizes = [
    { width: 320, height: 568, keyboardHeight: 340 },
    { width: 390, height: 844, keyboardHeight: 500 },
    { width: 844, height: 390, keyboardHeight: 310 },
  ]

  for (const size of sizes) {
    const safeArea = size.width === 320
      ? { top: 20, right: 8, bottom: 24, left: 8 }
      : zeroSafeArea
    await page.setViewportSize({ width: size.width, height: size.height })
    await setSafeAreaInsets(page, safeArea)
    await setMockVisualViewport(page, {
      width: size.width,
      height: size.height,
      offsetLeft: 0,
      offsetTop: 0,
    })

    if (size.width < 1024) {
      await page.getByRole('button', { name: 'Toggle session drawer' }).click()
      const drawer = page.getByRole('dialog', { name: 'Workspace sessions' })
      await expectInsideVisualViewport(
        drawer,
        `${size.width}×${size.height} Drawer`,
        safeArea,
        { left: true, right: true },
      )
      if (size.width === 320) {
        await expect.poll(() => drawer.evaluate(element => {
          const style = getComputedStyle(element)
          return {
            paddingTop: style.paddingTop,
            paddingBottom: style.paddingBottom,
          }
        })).toEqual({ paddingTop: '20px', paddingBottom: '24px' })
      }
      await page.keyboard.press('Escape')
      await expect(drawer).toBeHidden()
    } else {
      await expectInsideVisualViewport(
        page.locator('aside[aria-label="Workspace sessions"]'),
        `${size.width}×${size.height} Sidebar`,
      )
    }

    await page.keyboard.press('Meta+k')
    const palette = page.getByRole('dialog', { name: 'Command Palette' })
    await expectInsideVisualViewport(
      palette,
      `${size.width}×${size.height} Palette`,
      safeArea,
      { top: true, right: true, bottom: true, left: true },
    )
    await page.getByRole('button', { name: 'Close Command Palette' }).click()

    const toolbar = page.getByRole('toolbar', { name: 'Terminal keys' })
    await expectInsideVisualViewport(
      toolbar,
      `${size.width}×${size.height} Terminal toolbar`,
      safeArea,
      { right: true, left: true },
    )
    const textarea = await expandComposer(page)
    const sendButton = page.getByRole('button', { name: /^Send command to / })
    const sendSize = await sendButton.evaluate(element => {
      const rect = element.getBoundingClientRect()
      return { width: rect.width, height: rect.height }
    })
    expect(sendSize.width).toBeGreaterThanOrEqual(44)
    expect(sendSize.height).toBeGreaterThanOrEqual(44)

    await page.getByRole('button', { name: 'Hide terminal keys' }).click()
    await textarea.focus()
    await setMockVisualViewport(page, {
      width: size.width - 8,
      height: size.keyboardHeight,
      offsetLeft: 4,
      offsetTop: 8,
    })
    await expectInsideVisualViewport(
      page.getByRole('button', { name: 'Show terminal keys' }),
      `${size.width}×${size.height} collapsed toolbar with software keyboard`,
      safeArea,
      { right: true, bottom: true, left: true },
    )
    await expectInsideVisualViewport(
      page.getByRole('button', { name: 'Collapse Mobile Input Composer' }).locator('..'),
      `${size.width}×${size.height} Composer with software keyboard`,
      safeArea,
      { right: true, bottom: true, left: true },
    )

    if (size.width < 1024) {
      await page.getByRole('button', { name: 'Toggle session drawer' }).click()
      const drawer = page.getByRole('dialog', { name: 'Workspace sessions' })
      await expectInsideVisualViewport(
        drawer,
        `${size.width}×${size.height} Drawer with software keyboard`,
        safeArea,
        { left: true, right: true },
      )
      await page.keyboard.press('Escape')
      await expect(drawer).toBeHidden()
    }

    await page.keyboard.press('Meta+k')
    await expectInsideVisualViewport(
      page.getByRole('dialog', { name: 'Command Palette' }),
      `${size.width}×${size.height} Palette with software keyboard`,
      safeArea,
      { top: true, right: true, bottom: true, left: true },
    )
    await page.getByRole('button', { name: 'Close Command Palette' }).click()
    await expectNoShellOverflow(page)

    await setMockVisualViewport(page, {
      width: size.width,
      height: size.height,
      offsetLeft: 0,
      offsetTop: 0,
    })
    await page.getByRole('button', { name: 'Show terminal keys' }).click()
  }

  await page.setViewportSize({ width: 390, height: 844 })
  await setSafeAreaInsets(page, zeroSafeArea)
  await setMockVisualViewport(page, {
    width: 390,
    height: 844,
    offsetLeft: 0,
    offsetTop: 0,
  })
  const results = await new AxeBuilder({ page }).analyze()
  const blockingViolations = results.violations.filter(violation => (
    violation.impact === 'critical' || violation.impact === 'serious'
  ))
  expect(
    blockingViolations,
    'Mobile Terminal must have no critical or serious Axe violations',
  ).toEqual([])
})

const DEFAULT_NOTIFICATION = {
  title: 'TmuxAtlas',
  body: 'A TmuxAtlas session needs your attention.',
  url: '/',
}

self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting())
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

function safeDestination(value) {
  if (typeof value !== 'string' || value.length === 0) return '/'

  try {
    const destination = new URL(value, self.location.origin)
    if (destination.origin !== self.location.origin) return '/'
    return `${destination.pathname}${destination.search}${destination.hash}`
  } catch {
    return '/'
  }
}

function sessionDestination(payload) {
  if (typeof payload.url === 'string') return safeDestination(payload.url)
  if (typeof payload.session !== 'string' || payload.session.length === 0) return '/'

  const session = encodeURIComponent(payload.session)
  if (typeof payload.host === 'string' && payload.host.length > 0) {
    return `/session/${encodeURIComponent(payload.host)}/${session}`
  }
  return `/session/${session}`
}

function parsePushPayload(event) {
  if (!event.data) return {}

  try {
    const payload = event.data.json()
    return payload && typeof payload === 'object' ? payload : {}
  } catch {
    try {
      const payload = JSON.parse(event.data.text())
      return payload && typeof payload === 'object' ? payload : {}
    } catch {
      return {}
    }
  }
}

self.addEventListener('push', (event) => {
  const payload = parsePushPayload(event)
  const title = typeof payload.title === 'string' && payload.title.length > 0
    ? payload.title
    : DEFAULT_NOTIFICATION.title
  const body = typeof payload.body === 'string' && payload.body.length > 0
    ? payload.body
    : DEFAULT_NOTIFICATION.body
  const url = sessionDestination(payload)
  const tagParts = [payload.tool, payload.host, payload.session, payload.window]
    .filter((part) => typeof part === 'string' || typeof part === 'number')

  event.waitUntil(self.registration.showNotification(title, {
    body,
    icon: '/icon-192.png',
    badge: '/favicon-32.png',
    tag: tagParts.length > 0 ? tagParts.join(':') : undefined,
    data: { url },
  }))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const destination = safeDestination(event.notification.data?.url ?? DEFAULT_NOTIFICATION.url)

  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({
      type: 'window',
      includeUncontrolled: true,
    })

    for (const client of windows) {
      let clientOrigin
      try {
        clientOrigin = new URL(client.url).origin
      } catch {
        continue
      }
      if (clientOrigin !== self.location.origin) continue

      if ('navigate' in client) {
        await client.navigate(destination)
      }
      return client.focus()
    }

    return self.clients.openWindow(destination)
  })())
})

let csrfToken = ''
const browserFetch = window.fetch.bind(window)

export function setCSRFToken(token: string | null | undefined) {
  csrfToken = token || ''
}

export async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const method = (init.method || 'GET').toUpperCase()
  const headers = new Headers(init.headers)
  if (init.body != null && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) {
    headers.set('X-TmuxAtlas-CSRF', csrfToken)
  }
  const response = await browserFetch(input, { ...init, headers })
  if (response.status === 401) {
    const url = typeof input === 'string' ? input : input instanceof Request ? input.url : input.toString()
    if (!url.includes('/api/auth/')) {
      window.dispatchEvent(new Event('auth:unauthorized'))
    }
  }
  return response
}

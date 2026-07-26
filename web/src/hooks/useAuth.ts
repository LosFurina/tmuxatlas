import { useState, useEffect, useCallback } from 'react'

interface AuthState {
  loading: boolean
  authRequired: boolean
  needsSetup: boolean
  authenticated: boolean
  error: string | null
  rpId: string | null
  origin: string | null
  setup: (setupToken: string, label?: string) => Promise<boolean>
  login: () => Promise<boolean>
  logout: () => Promise<void>
}

let interceptorInstalled = false
function installFetchInterceptor() {
  if (interceptorInstalled) return
  interceptorInstalled = true
  const originalFetch = window.fetch
  window.fetch = async (...args) => {
    const res = await originalFetch(...args)
    if (res.status === 401) {
      const url = typeof args[0] === 'string' ? args[0] : (args[0] as Request).url
      if (!url.includes('/api/auth/')) {
        window.dispatchEvent(new Event('auth:unauthorized'))
      }
    }
    return res
  }
}

function decodeBase64URL(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

function encodeBase64URL(value: ArrayBuffer | null): string | null {
  if (value === null) return null
  const bytes = new Uint8Array(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function creationOptions(value: Record<string, unknown>): CredentialCreationOptions {
  const options = value.publicKey as unknown as {
    challenge: string
    user: Omit<PublicKeyCredentialUserEntity, 'id'> & { id: string }
    excludeCredentials?: Array<Omit<PublicKeyCredentialDescriptor, 'id'> & { id: string }>
    [key: string]: unknown
  }
  return {
    publicKey: {
      ...options,
      challenge: decodeBase64URL(options.challenge),
      user: { ...options.user, id: decodeBase64URL(options.user.id) },
      excludeCredentials: options.excludeCredentials?.map((item) => ({
        ...item,
        id: decodeBase64URL(item.id),
      })),
    } as PublicKeyCredentialCreationOptions,
  }
}

function requestOptions(value: Record<string, unknown>): CredentialRequestOptions {
  const options = value.publicKey as unknown as {
    challenge: string
    allowCredentials?: Array<Omit<PublicKeyCredentialDescriptor, 'id'> & { id: string }>
    [key: string]: unknown
  }
  return {
    publicKey: {
      ...options,
      challenge: decodeBase64URL(options.challenge),
      allowCredentials: options.allowCredentials?.map((item) => ({
        ...item,
        id: decodeBase64URL(item.id),
      })),
    } as PublicKeyCredentialRequestOptions,
  }
}

async function responseError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json()
    return body.error || fallback
  } catch {
    return fallback
  }
}

async function registerPasskey(setupToken: string, label: string): Promise<void> {
  const begin = await fetch('/api/auth/passkey/register/begin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ setup_token: setupToken, label }),
  })
  if (!begin.ok) throw new Error(await responseError(begin, 'Could not start passkey setup'))

  const credential = await navigator.credentials.create(
    creationOptions(await begin.json()),
  ) as PublicKeyCredential | null
  if (!credential) throw new Error('Passkey creation was cancelled')
  const response = credential.response as AuthenticatorAttestationResponse
  const transports = typeof response.getTransports === 'function' ? response.getTransports() : []

  const finish = await fetch('/api/auth/passkey/register/finish', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: credential.id,
      rawId: encodeBase64URL(credential.rawId),
      type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment,
      response: {
        clientDataJSON: encodeBase64URL(response.clientDataJSON),
        attestationObject: encodeBase64URL(response.attestationObject),
        transports,
      },
      clientExtensionResults: credential.getClientExtensionResults(),
    }),
  })
  if (!finish.ok) throw new Error(await responseError(finish, 'Passkey verification failed'))
}

async function authenticatePasskey(): Promise<void> {
  const begin = await fetch('/api/auth/passkey/login/begin', { method: 'POST' })
  if (!begin.ok) throw new Error(await responseError(begin, 'Could not start passkey sign-in'))

  const credential = await navigator.credentials.get(
    requestOptions(await begin.json()),
  ) as PublicKeyCredential | null
  if (!credential) throw new Error('Passkey sign-in was cancelled')
  const response = credential.response as AuthenticatorAssertionResponse

  const finish = await fetch('/api/auth/passkey/login/finish', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id: credential.id,
      rawId: encodeBase64URL(credential.rawId),
      type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment,
      response: {
        clientDataJSON: encodeBase64URL(response.clientDataJSON),
        authenticatorData: encodeBase64URL(response.authenticatorData),
        signature: encodeBase64URL(response.signature),
        userHandle: encodeBase64URL(response.userHandle),
      },
      clientExtensionResults: credential.getClientExtensionResults(),
    }),
  })
  if (!finish.ok) throw new Error(await responseError(finish, 'Passkey verification failed'))
}

export function useAuth(): AuthState {
  const [loading, setLoading] = useState(true)
  const [authRequired, setAuthRequired] = useState(false)
  const [needsSetup, setNeedsSetup] = useState(false)
  const [authenticated, setAuthenticated] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [rpId, setRPID] = useState<string | null>(null)
  const [origin, setOrigin] = useState<string | null>(null)

  useEffect(() => {
    installFetchInterceptor()

    async function checkAuth() {
      try {
        const statusRes = await fetch('/api/auth/status')
        const status = await statusRes.json()
        setRPID(status.rp_id || null)
        setOrigin(status.origin || null)
        if (!status.auth_required) {
          setAuthenticated(true)
          setLoading(false)
          return
        }
        setAuthRequired(true)
        if (status.needs_setup) {
          setNeedsSetup(true)
          setLoading(false)
          return
        }
        const checkRes = await fetch('/api/auth/check')
        const check = await checkRes.json()
        setAuthenticated(check.authenticated === true)
      } catch {
        setAuthenticated(false)
      }
      setLoading(false)
    }
    checkAuth()
  }, [])

  useEffect(() => {
    if (!authRequired) return
    const onUnauthorized = () => setAuthenticated(false)
    window.addEventListener('auth:unauthorized', onUnauthorized)
    return () => window.removeEventListener('auth:unauthorized', onUnauthorized)
  }, [authRequired])

  const ensureSupported = () => {
    if (!window.isSecureContext) throw new Error('Passkeys require HTTPS (localhost is allowed for local testing)')
    if (!window.PublicKeyCredential) throw new Error('This browser does not support passkeys')
  }

  const setup = useCallback(async (setupToken: string, label = ''): Promise<boolean> => {
    setError(null)
    try {
      ensureSupported()
      await registerPasskey(setupToken.trim(), label.trim())
      setNeedsSetup(false)
      setAuthenticated(true)
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Passkey setup failed')
      return false
    }
  }, [])

  const login = useCallback(async (): Promise<boolean> => {
    setError(null)
    try {
      ensureSupported()
      await authenticatePasskey()
      setAuthenticated(true)
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Passkey sign-in failed')
      return false
    }
  }, [])

  const logout = useCallback(async () => {
    try {
      await fetch('/api/auth/logout', { method: 'POST' })
    } catch {
      // The local UI should still return to the sign-in screen.
    }
    setAuthenticated(false)
  }, [])

  return {
    loading, authRequired, needsSetup, authenticated, error,
    rpId, origin, setup, login, logout,
  }
}

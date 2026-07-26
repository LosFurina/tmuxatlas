import { apiFetch } from './api'

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

export async function responseError(res: Response, fallback: string): Promise<string> {
  try {
    const body = await res.json()
    return body.error || fallback
  } catch {
    return fallback
  }
}

export function ensurePasskeysSupported() {
  if (!window.isSecureContext) throw new Error('Passkeys require HTTPS (localhost is allowed for local testing)')
  if (!window.PublicKeyCredential) throw new Error('This browser does not support passkeys')
}

export async function registerPasskey(setupToken = '', label = ''): Promise<void> {
  ensurePasskeysSupported()
  const begin = await apiFetch('/api/auth/passkey/register/begin', {
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

  const finish = await apiFetch('/api/auth/passkey/register/finish', {
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

export async function authenticatePasskey(): Promise<void> {
  ensurePasskeysSupported()
  const begin = await apiFetch('/api/auth/passkey/login/begin', {
    method: 'POST',
    body: JSON.stringify({}),
  })
  if (!begin.ok) throw new Error(await responseError(begin, 'Could not start passkey sign-in'))

  const credential = await navigator.credentials.get(
    requestOptions(await begin.json()),
  ) as PublicKeyCredential | null
  if (!credential) throw new Error('Passkey sign-in was cancelled')
  const response = credential.response as AuthenticatorAssertionResponse

  const finish = await apiFetch('/api/auth/passkey/login/finish', {
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

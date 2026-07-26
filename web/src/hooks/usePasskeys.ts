import { useCallback, useEffect, useState } from 'react'
import { registerPasskey, responseError } from '../lib/webauthn'

export interface PasskeyMetadata {
  id: string
  label: string
  created_at: string
  last_used_at?: string
}

export function usePasskeys() {
  const [passkeys, setPasskeys] = useState<PasskeyMetadata[]>([])
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const response = await fetch('/api/auth/passkeys')
      if (!response.ok) throw new Error(await responseError(response, 'Could not load passkeys'))
      const body = await response.json()
      setPasskeys(body.passkeys || [])
      setError(null)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load passkeys')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const add = useCallback(async (label: string) => {
    setPending('add')
    setError(null)
    try {
      await registerPasskey('', label.trim())
      await refresh()
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not add passkey')
      return false
    } finally {
      setPending(null)
    }
  }, [refresh])

  const rename = useCallback(async (id: string, label: string) => {
    setPending(id)
    setError(null)
    try {
      const response = await fetch(`/api/auth/passkeys/${encodeURIComponent(id)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label }),
      })
      if (!response.ok) throw new Error(await responseError(response, 'Could not rename passkey'))
      await refresh()
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not rename passkey')
      return false
    } finally {
      setPending(null)
    }
  }, [refresh])

  const remove = useCallback(async (id: string) => {
    setPending(id)
    setError(null)
    try {
      const response = await fetch(`/api/auth/passkeys/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!response.ok) throw new Error(await responseError(response, 'Could not delete passkey'))
      await refresh()
      return true
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete passkey')
      await refresh()
      return false
    } finally {
      setPending(null)
    }
  }, [refresh])

  return { passkeys, loading, pending, error, refresh, add, rename, remove }
}

import { useState, useEffect, useCallback } from 'react'

type PushState = 'unsupported' | 'prompt' | 'granted' | 'denied' | 'subscribed'

const serviceWorkerURL = '/sw.js'
const serviceWorkerScope = '/'

function urlBase64ToUint8Array(base64String: string): ArrayBuffer {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  const array = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i++) {
    array[i] = raw.charCodeAt(i)
  }
  return array.buffer as ArrayBuffer
}

export function usePushNotifications() {
  const [state, setState] = useState<PushState>('unsupported')

  const getRegistration = useCallback(() => {
    return navigator.serviceWorker.getRegistration(serviceWorkerScope)
  }, [])

  useEffect(() => {
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
      setState('unsupported')
      return
    }

    // Check current permission + subscription state
    const check = async () => {
      const permission = Notification.permission
      if (permission === 'denied') {
        setState('denied')
        return
      }

      try {
        const reg = await getRegistration()
        if (reg) {
          const sub = await reg.pushManager.getSubscription()
          if (sub) {
            // Re-register with server (subscriptions are in-memory, lost on restart)
            fetch('/api/push/subscribe', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(sub.toJSON()),
            }).catch(() => {})
            setState('subscribed')
            return
          }
        }
      } catch {}

      setState(permission === 'granted' ? 'granted' : 'prompt')
    }

    check()
  }, [getRegistration])

  const subscribe = useCallback(async () => {
    try {
      const reg = await navigator.serviceWorker.register(serviceWorkerURL, {
        scope: serviceWorkerScope,
        updateViaCache: 'none',
      })
      const ready = await navigator.serviceWorker.ready
      const activeRegistration = ready.scope === reg.scope ? ready : reg

      // Get VAPID public key from server
      const res = await fetch('/api/push/vapid-key')
      if (!res.ok) {
        console.error('Failed to get VAPID key')
        return false
      }
      const { public_key } = await res.json()

      // Subscribe to push
      const sub = await activeRegistration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      })

      // Send subscription to server
      const sendRes = await fetch('/api/push/subscribe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(sub.toJSON()),
      })

      if (sendRes.ok) {
        setState('subscribed')
        return true
      }
    } catch (err) {
      console.error('Push subscription failed:', err)
      if (Notification.permission === 'denied') {
        setState('denied')
      }
    }
    return false
  }, [])

  const unsubscribe = useCallback(async () => {
    try {
      const reg = await getRegistration()
      if (reg) {
        const sub = await reg.pushManager.getSubscription()
        if (sub) {
          // Notify server
          await fetch('/api/push/unsubscribe', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ endpoint: sub.endpoint }),
          })
          await sub.unsubscribe()
        }
      }
      setState('prompt')
    } catch (err) {
      console.error('Push unsubscribe failed:', err)
    }
  }, [getRegistration])

  return { pushState: state, subscribe, unsubscribe }
}

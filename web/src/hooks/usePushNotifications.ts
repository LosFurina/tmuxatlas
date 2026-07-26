import { useState, useEffect, useCallback, useRef } from 'react'

export type PushState =
  | 'unsupported'
  | 'prompt'
  | 'granted'
  | 'denied'
  | 'syncing'
  | 'subscribed'
  | 'error'

const serviceWorkerURL = '/sw.js'
const serviceWorkerScope = '/'

function urlBase64ToUint8Array(base64String: string): ArrayBuffer {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  const array = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i++) array[i] = raw.charCodeAt(i)
  return array.buffer as ArrayBuffer
}

export async function registerPushWithHub(
  subscription: PushSubscription,
  fetcher: typeof fetch = fetch,
): Promise<boolean> {
  const response = await fetcher('/api/push/subscribe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(subscription.toJSON()),
  })
  return response.ok
}

export async function unsubscribePushEverywhere(
  subscription: PushSubscription,
  fetcher: typeof fetch = fetch,
): Promise<{ hub: boolean; browser: boolean }> {
  const response = await fetcher('/api/push/unsubscribe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint: subscription.endpoint }),
  })
  if (!response.ok) return { hub: false, browser: true }
  return { hub: true, browser: await subscription.unsubscribe() }
}

export function usePushNotifications() {
  const [state, setState] = useState<PushState>('unsupported')
  const generationRef = useRef(0)

  const getRegistration = useCallback(() => (
    navigator.serviceWorker.getRegistration(serviceWorkerScope)
  ), [])

  const reconcile = useCallback(async () => {
    const generation = ++generationRef.current
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
      setState('unsupported')
      return false
    }
    if (Notification.permission === 'denied') {
      setState('denied')
      return false
    }

    setState('syncing')
    try {
      const registration = await getRegistration()
      const subscription = await registration?.pushManager.getSubscription()
      if (subscription) {
        const persisted = await registerPushWithHub(subscription)
        if (generation !== generationRef.current) return false
        setState(persisted ? 'subscribed' : 'error')
        return persisted
      }
      if (generation === generationRef.current) {
        setState(Notification.permission === 'granted' ? 'granted' : 'prompt')
      }
    } catch {
      if (generation === generationRef.current) setState('error')
    }
    return false
  }, [getRegistration])

  useEffect(() => {
    void reconcile()
    return () => { generationRef.current++ }
  }, [reconcile])

  const subscribe = useCallback(async () => {
    const generation = ++generationRef.current
    setState('syncing')
    try {
      const registration = await navigator.serviceWorker.register(serviceWorkerURL, {
        scope: serviceWorkerScope,
        updateViaCache: 'none',
      })
      const ready = await navigator.serviceWorker.ready
      const activeRegistration = ready.scope === registration.scope ? ready : registration
      const response = await fetch('/api/push/vapid-key')
      if (!response.ok) throw new Error('Failed to get VAPID key')
      const { public_key } = await response.json()
      const subscription = await activeRegistration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      })
      const persisted = await registerPushWithHub(subscription)
      if (generation !== generationRef.current) return false
      setState(persisted ? 'subscribed' : 'error')
      return persisted
    } catch {
      if (generation === generationRef.current) {
        setState(Notification.permission === 'denied' ? 'denied' : 'error')
      }
      return false
    }
  }, [])

  const unsubscribe = useCallback(async () => {
    const generation = ++generationRef.current
    setState('syncing')
    try {
      const registration = await getRegistration()
      const subscription = await registration?.pushManager.getSubscription()
      if (!subscription) {
        setState('prompt')
        return true
      }
      const result = await unsubscribePushEverywhere(subscription)
      if (generation !== generationRef.current) return false
      const complete = result.hub && result.browser
      setState(complete ? 'prompt' : 'error')
      return complete
    } catch {
      if (generation === generationRef.current) setState('error')
      return false
    }
  }, [getRegistration])

  return {
    pushState: state,
    subscribe,
    unsubscribe,
    retry: reconcile,
  }
}

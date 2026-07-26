import { describe, expect, it, vi } from 'vitest'
import {
  registerPushWithHub,
  unsubscribePushEverywhere,
} from './usePushNotifications'

function subscription(unsubscribe = vi.fn(async () => true)): PushSubscription {
  return {
    endpoint: 'https://push.example/subscription',
    expirationTime: null,
    options: {} as PushSubscriptionOptions,
    getKey: () => null,
    toJSON: () => ({ endpoint: 'https://push.example/subscription' }),
    unsubscribe,
  }
}

describe('Push reconciliation', () => {
  it('reports subscribed only after Hub persistence succeeds and supports retry', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 503 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const local = subscription()
    expect(await registerPushWithHub(local, fetcher)).toBe(false)
    expect(await registerPushWithHub(local, fetcher)).toBe(true)
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('does not remove the browser subscription when Hub unsubscribe fails', async () => {
    const browserUnsubscribe = vi.fn(async () => true)
    const result = await unsubscribePushEverywhere(
      subscription(browserUnsubscribe),
      vi.fn(async () => new Response(null, { status: 500 })),
    )
    expect(result).toEqual({ hub: false, browser: true })
    expect(browserUnsubscribe).not.toHaveBeenCalled()
  })

  it('reports a partial browser failure after Hub removal', async () => {
    const result = await unsubscribePushEverywhere(
      subscription(vi.fn(async () => false)),
      vi.fn(async () => new Response(null, { status: 204 })),
    )
    expect(result).toEqual({ hub: true, browser: false })
  })
})

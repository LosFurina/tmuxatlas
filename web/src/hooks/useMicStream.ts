import { useCallback, useEffect, useRef, useState } from 'react'
import { apiFetch } from '../lib/api'
import { createPcmEncoder } from '../lib/micPcm'

export type MicState = 'off' | 'starting' | 'on'

interface VoiceStatus {
  enabled: boolean
  ready: boolean
  error?: string
}

const CAPTURE_WORKLET = `
class TmuxAtlasMicCapture extends AudioWorkletProcessor {
  process(inputs) {
    const channel = inputs[0] && inputs[0][0]
    if (channel) this.port.postMessage(channel.slice(0))
    return true
  }
}
registerProcessor('tmuxatlas-mic-capture', TmuxAtlasMicCapture)
`

// Past this much unsent audio (~1 s), drop frames: late dictation is worse than a gap.
const MAX_BUFFERED_BYTES = 32 * 1024

function micErrorMessage(error: unknown): string {
  if (!window.isSecureContext) return 'The microphone needs HTTPS or localhost.'
  if (error instanceof DOMException) {
    if (error.name === 'NotAllowedError') return 'Microphone access was denied in this browser.'
    if (error.name === 'NotFoundError') return 'This device has no microphone.'
  }
  return error instanceof Error ? error.message : 'The microphone could not be started.'
}

async function fetchVoiceStatus(): Promise<VoiceStatus> {
  const response = await apiFetch('/api/voice')
  if (!response.ok) return { enabled: false, ready: false }
  return await response.json() as VoiceStatus
}

/**
 * Streams the browser microphone to the host's virtual microphone. Nothing is
 * captured until start() and all capture stops on stop(), failure or unmount.
 */
export function useMicStream(allowed: boolean) {
  const [enabled, setEnabled] = useState(false)
  const [state, setState] = useState<MicState>('off')
  const [error, setError] = useState('')
  const stopRef = useRef<(() => void) | null>(null)
  const attemptRef = useRef(0)

  useEffect(() => {
    if (!allowed) return
    let cancelled = false
    fetchVoiceStatus()
      .then(status => { if (!cancelled) setEnabled(status.enabled) })
      .catch(() => { if (!cancelled) setEnabled(false) })
    return () => { cancelled = true }
  }, [allowed])

  const stop = useCallback(() => {
    attemptRef.current += 1
    stopRef.current?.()
    stopRef.current = null
    setState('off')
  }, [])

  const start = useCallback(async () => {
    const attempt = ++attemptRef.current
    const isCurrent = () => attemptRef.current === attempt
    const cleanups: (() => void)[] = []
    const cleanup = () => {
      for (const run of cleanups.splice(0).reverse()) {
        try { run() } catch { /* already released */ }
      }
    }
    const fail = (message: string) => {
      cleanup()
      if (!isCurrent()) return
      stopRef.current = null
      setError(message)
      setState('off')
    }

    setError('')
    setState('starting')
    stopRef.current = cleanup
    try {
      const status = await fetchVoiceStatus()
      if (!isCurrent()) return
      if (status.error) throw new Error(status.error)
      if (!navigator.mediaDevices?.getUserMedia) {
        throw new Error('The microphone needs HTTPS or localhost.')
      }

      const media = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
      })
      cleanups.push(() => media.getTracks().forEach(track => track.stop()))
      if (!isCurrent()) return cleanup()

      const context = new AudioContext()
      cleanups.push(() => void context.close())
      const moduleUrl = URL.createObjectURL(new Blob([CAPTURE_WORKLET], { type: 'text/javascript' }))
      try {
        await context.audioWorklet.addModule(moduleUrl)
      } finally {
        URL.revokeObjectURL(moduleUrl)
      }
      if (!isCurrent()) return cleanup()
      await context.resume()
      if (!isCurrent()) return cleanup()

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const socket = new WebSocket(`${protocol}//${window.location.host}/ws/mic`)
      socket.binaryType = 'arraybuffer'
      cleanups.push(() => {
        socket.onclose = null
        socket.close()
      })
      await new Promise<void>((resolve, reject) => {
        socket.onopen = () => resolve()
        socket.onerror = () => reject(new Error('The server refused the microphone stream.'))
      })
      if (!isCurrent()) return cleanup()

      const encoder = createPcmEncoder(context.sampleRate)
      const source = context.createMediaStreamSource(media)
      const capture = new AudioWorkletNode(context, 'tmuxatlas-mic-capture', { numberOfOutputs: 0 })
      capture.port.onmessage = (event: MessageEvent<Float32Array>) => {
        for (const frame of encoder.push(event.data)) {
          if (socket.readyState === WebSocket.OPEN && socket.bufferedAmount < MAX_BUFFERED_BYTES) {
            socket.send(frame.buffer)
          }
        }
      }
      source.connect(capture)
      cleanups.push(() => {
        capture.port.onmessage = null
        source.disconnect()
      })

      socket.onclose = event => {
        fail(event.reason || 'The microphone stream was interrupted.')
      }
      for (const track of media.getAudioTracks()) {
        track.onended = () => fail('The browser stopped the microphone.')
      }
      setState('on')
    } catch (startError) {
      fail(micErrorMessage(startError))
    }
  }, [])

  const toggle = useCallback(() => {
    if (state === 'off') void start()
    else stop()
  }, [start, state, stop])

  useEffect(() => {
    if (!allowed) stop()
  }, [allowed, stop])

  useEffect(() => stop, [stop])

  return { available: allowed && enabled, state, error, toggle }
}

/** Sample rate the server's virtual microphone expects, as 16-bit little-endian mono. */
export const MIC_SAMPLE_RATE = 16000
/** 20 ms at MIC_SAMPLE_RATE. */
export const MIC_FRAME_SAMPLES = 320

export interface PcmEncoder {
  /** Accepts float samples at the input rate and returns complete 20 ms PCM frames. */
  push: (samples: Float32Array) => Int16Array[]
}

/**
 * Downsamples by averaging the input samples that fall in each output sample,
 * which low-pass filters enough for speech. State carries across calls, so
 * chunk boundaries do not produce clicks or drift.
 */
export function createPcmEncoder(inputRate: number, outputRate = MIC_SAMPLE_RATE): PcmEncoder {
  if (!(inputRate >= outputRate)) {
    throw new Error(`Unsupported microphone sample rate: ${inputRate} Hz`)
  }
  const step = inputRate / outputRate
  let frame = new Int16Array(MIC_FRAME_SAMPLES)
  let frameLength = 0
  let position = 0
  let boundary = step
  let sum = 0
  let count = 0

  return {
    push(samples) {
      const frames: Int16Array[] = []
      for (const sample of samples) {
        sum += sample
        count += 1
        position += 1
        if (position < boundary) continue
        const mean = sum / count
        const clamped = Math.max(-1, Math.min(1, mean))
        frame[frameLength] = Math.round(clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff)
        frameLength += 1
        sum = 0
        count = 0
        // Rebase to keep the fractional boundary precise on long streams.
        position -= boundary
        boundary = step
        if (frameLength === MIC_FRAME_SAMPLES) {
          frames.push(frame)
          frame = new Int16Array(MIC_FRAME_SAMPLES)
          frameLength = 0
        }
      }
      return frames
    },
  }
}

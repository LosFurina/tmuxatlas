import { describe, expect, it } from 'vitest'
import { MIC_FRAME_SAMPLES, createPcmEncoder } from './micPcm'

function sine(rate: number, seconds: number, hz = 440, amplitude = 0.5): Float32Array {
  const samples = new Float32Array(Math.round(rate * seconds))
  for (let index = 0; index < samples.length; index++) {
    samples[index] = amplitude * Math.sin((2 * Math.PI * hz * index) / rate)
  }
  return samples
}

function pcm(frames: Int16Array[]): number[] {
  return frames.flatMap(frame => Array.from(frame))
}

function zeroCrossingsPerSecond(values: number[], rate: number): number {
  let crossings = 0
  for (let index = 1; index < values.length; index++) {
    if ((values[index - 1] < 0) !== (values[index] < 0)) crossings += 1
  }
  return crossings / 2 / (values.length / rate)
}

describe('createPcmEncoder', () => {
  it.each([16000, 44100, 48000, 96000])('preserves pitch and duration from %i Hz', rate => {
    const values = pcm(createPcmEncoder(rate).push(sine(rate, 1)))
    expect(Math.abs(values.length - 16000)).toBeLessThanOrEqual(MIC_FRAME_SAMPLES)
    expect(zeroCrossingsPerSecond(values, 16000)).toBeCloseTo(440, -1)
    expect(Math.max(...values)).toBeGreaterThan(0.45 * 0x7fff)
  })

  it('emits only complete 20 ms frames', () => {
    const encoder = createPcmEncoder(16000)
    expect(encoder.push(new Float32Array(319))).toEqual([])
    const frames = encoder.push(new Float32Array(1))
    expect(frames).toHaveLength(1)
    expect(frames[0]).toHaveLength(MIC_FRAME_SAMPLES)
  })

  it('does not depend on how the input is chunked', () => {
    const input = sine(44100, 0.5)
    const whole = pcm(createPcmEncoder(44100).push(input))
    const chunked = createPcmEncoder(44100)
    const pieces: Int16Array[] = []
    for (let start = 0; start < input.length; start += 128) {
      pieces.push(...chunked.push(input.subarray(start, start + 128)))
    }
    expect(pcm(pieces)).toEqual(whole)
  })

  it('does not drift on long streams', () => {
    const encoder = createPcmEncoder(44100)
    let produced = 0
    for (let second = 0; second < 600; second++) {
      produced += encoder.push(new Float32Array(44100)).length * MIC_FRAME_SAMPLES
    }
    expect(Math.abs(produced - 600 * 16000)).toBeLessThanOrEqual(MIC_FRAME_SAMPLES)
  })

  it('clamps out-of-range input', () => {
    const [frame] = createPcmEncoder(16000).push(new Float32Array(MIC_FRAME_SAMPLES).fill(2))
    expect(frame[0]).toBe(0x7fff)
    const [negative] = createPcmEncoder(16000).push(new Float32Array(MIC_FRAME_SAMPLES).fill(-2))
    expect(negative[0]).toBe(-0x8000)
  })

  it('rejects input slower than the output rate', () => {
    expect(() => createPcmEncoder(8000)).toThrow('8000')
  })
})

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createPreviewFpsCounter } from './previewFps'

function mockRfb() {
  const flip = vi.fn()
  return {
    _display: { flip },
    origFlip: flip,
  }
}

describe('createPreviewFpsCounter', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('counts flip calls and updates fps every second', () => {
    const counter = createPreviewFpsCounter()
    const rfb = mockRfb()
    counter.attach(rfb)

    rfb._display.flip()
    rfb._display.flip()
    rfb._display.flip()

    vi.advanceTimersByTime(1000)
    expect(counter.fps.value).toBe(3)
    expect(counter.hasRecentFrames.value).toBe(true)

    rfb._display.flip()
    vi.advanceTimersByTime(1000)
    expect(counter.fps.value).toBe(1)
    expect(counter.hasRecentFrames.value).toBe(true)

    vi.advanceTimersByTime(1000)
    expect(counter.fps.value).toBe(0)
    expect(counter.hasRecentFrames.value).toBe(false)

    counter.detach()
  })

  it('restores original flip on detach', () => {
    const counter = createPreviewFpsCounter()
    const calls: string[] = []
    const orig = () => {
      calls.push('orig')
    }
    const rfb = { _display: { flip: orig } }

    counter.attach(rfb)
    const wrapped = rfb._display.flip
    expect(wrapped).not.toBe(orig)

    wrapped()
    expect(calls).toEqual(['orig'])

    counter.detach()
    expect(rfb._display.flip).toBe(orig)
    rfb._display.flip()
    expect(calls).toEqual(['orig', 'orig'])
  })

  it('counts only flips within the last second when interval is delayed', () => {
    const counter = createPreviewFpsCounter()
    const rfb = mockRfb()
    counter.attach(rfb)

    for (let i = 0; i < 12; i++) rfb._display.flip()
    vi.advanceTimersByTime(3000)

    for (let i = 0; i < 12; i++) rfb._display.flip()
    vi.advanceTimersByTime(1000)

    expect(counter.fps.value).toBe(12)
    expect(counter.hasRecentFrames.value).toBe(true)

    counter.detach()
  })

  it('reset clears fps and hasRecentFrames', () => {
    const counter = createPreviewFpsCounter()
    const rfb = mockRfb()
    counter.attach(rfb)

    rfb._display.flip()
    vi.advanceTimersByTime(1000)
    expect(counter.fps.value).toBe(1)

    counter.reset()
    expect(counter.fps.value).toBe(0)
    expect(counter.hasRecentFrames.value).toBe(false)

    counter.detach()
  })

  it('ignores attach when _display.flip is missing', () => {
    const counter = createPreviewFpsCounter()
    counter.attach({})
    vi.advanceTimersByTime(2000)
    expect(counter.fps.value).toBe(0)
    expect(counter.hasRecentFrames.value).toBe(false)
  })

  it('delegates to original flip implementation', () => {
    const counter = createPreviewFpsCounter()
    const origFlip = vi.fn(() => 'ok')
    const rfb = { _display: { flip: origFlip } }

    counter.attach(rfb)
    const result = (rfb._display.flip as () => string)()
    expect(origFlip).toHaveBeenCalled()
    expect(result).toBe('ok')

    counter.detach()
  })
})

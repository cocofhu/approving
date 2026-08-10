import { describe, expect, it, vi } from 'vitest'
import { createDelayedBusy, useDelayedBusy } from './useDelayedBusy'

describe('useDelayedBusy / createDelayedBusy', () => {
  it('does not show UI when busy ends before 200ms (initial)', async () => {
    vi.useFakeTimers()
    const d = createDelayedBusy({ mode: 'initial' })
    d.setBusy(true)
    expect(d.showUi.value).toBe(false)
    await vi.advanceTimersByTimeAsync(199)
    expect(d.showUi.value).toBe(false)
    d.setBusy(false)
    await vi.advanceTimersByTimeAsync(50)
    expect(d.showUi.value).toBe(false)
    vi.useRealTimers()
  })

  it('shows after 200ms and honors minVisible=300ms', async () => {
    vi.useFakeTimers()
    const d = createDelayedBusy({ mode: 'initial' })
    d.setBusy(true)
    await vi.advanceTimersByTimeAsync(200)
    expect(d.showUi.value).toBe(true)
    d.setBusy(false)
    await vi.advanceTimersByTimeAsync(299)
    expect(d.showUi.value).toBe(true)
    await vi.advanceTimersByTimeAsync(1)
    expect(d.showUi.value).toBe(false)
    vi.useRealTimers()
  })

  it('submit mode shows immediately (0ms)', async () => {
    vi.useFakeTimers()
    const d = createDelayedBusy({ mode: 'submit' })
    d.setBusy(true)
    expect(d.showUi.value).toBe(true)
    d.setBusy(false)
    expect(d.showUi.value).toBe(false)
    vi.useRealTimers()
  })

  it('refresh uses the same 200/300 thresholds as initial', async () => {
    vi.useFakeTimers()
    const d = createDelayedBusy({ mode: 'refresh' })
    d.setBusy(true)
    await vi.advanceTimersByTimeAsync(199)
    expect(d.showUi.value).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    expect(d.showUi.value).toBe(true)
    d.setBusy(false)
    await vi.advanceTimersByTimeAsync(300)
    expect(d.showUi.value).toBe(false)
    vi.useRealTimers()
  })

  it('allows overriding showAfter and minVisible', async () => {
    vi.useFakeTimers()
    const d = createDelayedBusy({ mode: 'initial', showAfterMs: 50, minVisibleMs: 80 })
    d.setBusy(true)
    await vi.advanceTimersByTimeAsync(49)
    expect(d.showUi.value).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    expect(d.showUi.value).toBe(true)
    d.setBusy(false)
    await vi.advanceTimersByTimeAsync(80)
    expect(d.showUi.value).toBe(false)
    vi.useRealTimers()
  })

  it('reset immediately clears showUi even during minVisible', async () => {
    vi.useFakeTimers()
    const d = createDelayedBusy({ mode: 'initial' })
    d.setBusy(true)
    await vi.advanceTimersByTimeAsync(200)
    expect(d.showUi.value).toBe(true)
    d.reset()
    expect(d.showUi.value).toBe(false)
    expect(d.busy.value).toBe(false)
    vi.useRealTimers()
  })

  it('useDelayedBusy works outside setup (no onUnmounted)', () => {
    const d = useDelayedBusy({ mode: 'submit' })
    d.start()
    expect(d.showUi.value).toBe(true)
    d.stop()
    expect(d.showUi.value).toBe(false)
  })
})

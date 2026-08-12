// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import {
  PLATFORM_STATUS_POLL_MS,
  usePlatformStatusMetrics,
} from './usePlatformStatusMetrics'

const platformStatus = vi.fn()

vi.mock('@/lib/api/api', () => ({
  api: {
    platformStatus: (...args: unknown[]) => platformStatus(...args),
  },
}))

function mountHook() {
  let api: ReturnType<typeof usePlatformStatusMetrics> | undefined
  const Comp = defineComponent({
    setup() {
      api = usePlatformStatusMetrics()
      return () => h('div')
    },
  })
  const w = mount(Comp)
  return { w, api: () => api! }
}

describe('usePlatformStatusMetrics', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    platformStatus.mockReset()
    platformStatus.mockResolvedValue({
      cumulativeTokens: 10,
      current5mBucketTokens: 1,
      todayMaxCompleted5mTokens: 2,
      runningCount: 0,
      queuedCount: 0,
      asOf: '2026-08-12T00:00:00Z',
      timezone: 'UTC',
    })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('polls every 12s while visible and pauses when hidden', async () => {
    const { w, api } = mountHook()
    await flushPromises()
    expect(api().metrics.value?.cumulativeTokens).toBe(10)
    expect(platformStatus).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(PLATFORM_STATUS_POLL_MS)
    await flushPromises()
    expect(platformStatus).toHaveBeenCalledTimes(2)

    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    document.dispatchEvent(new Event('visibilitychange'))
    vi.advanceTimersByTime(PLATFORM_STATUS_POLL_MS * 2)
    await flushPromises()
    expect(platformStatus).toHaveBeenCalledTimes(2)

    w.unmount()
  })

  it('retains lastSuccess when refresh fails', async () => {
    const { w, api } = mountHook()
    await flushPromises()
    platformStatus.mockRejectedValueOnce(new Error('boom'))
    vi.advanceTimersByTime(PLATFORM_STATUS_POLL_MS)
    await flushPromises()
    expect(api().metrics.value?.cumulativeTokens).toBe(10)
    expect(api().stale.value).toBe(true)
    w.unmount()
  })
})

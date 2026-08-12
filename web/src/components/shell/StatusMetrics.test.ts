// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { nextTick, ref } from 'vue'
import shell from '@/locales/zh-CN/shell.json'
import { PLATFORM_STATUS_POLL_MS } from '@/lib/composables/usePlatformStatusMetrics'
import StatusMetrics from './StatusMetrics.vue'

const platformStatus = vi.fn()
const isMobile = ref(false)

vi.mock('@/lib/api/api', () => ({
  api: {
    platformStatus: (...args: unknown[]) => platformStatus(...args),
  },
}))

vi.mock('@/lib/composables/useBreakpoint', () => ({
  useBreakpoint: () => ({ isMobile }),
}))

function mountMetrics() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...shell } },
  })
  return mount(StatusMetrics, {
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
}

describe('StatusMetrics', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    isMobile.value = false
    platformStatus.mockReset()
    platformStatus.mockResolvedValue({
      cumulativeTokens: 1240582,
      current5mBucketTokens: 4812,
      todayMaxCompleted5mTokens: 12104,
      runningCount: 3,
      queuedCount: 5,
      currentBucketStart: '2026-08-12T06:05:00Z',
      currentBucketEnd: '2026-08-12T06:10:00Z',
      peakBucketStart: '2026-08-12T03:20:00Z',
      peakBucketEnd: '2026-08-12T03:25:00Z',
      asOf: '2026-08-12T06:07:00Z',
      timezone: 'Asia/Shanghai',
    })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('renders five desktop metrics with /5m rate (plan g2.3)', async () => {
    const w = mountMetrics()
    await flushPromises()
    expect(w.find('[data-testid="status-metrics"]').exists()).toBe(true)
    expect(w.find('[data-testid="status-metrics-tokens"]').text()).toContain('1.24M')
    expect(w.find('[data-testid="status-metrics-rate"]').text()).toMatch(/4\.8K\/5m/i)
    expect(w.find('[data-testid="status-metrics-peak"]').text()).toContain('12.1K')
    expect(w.find('[data-testid="status-metrics-running"]').text()).toContain('3')
    expect(w.find('[data-testid="status-metrics-queued"]').text()).toContain('5')
    w.unmount()
  })

  it('keeps lastSuccess on failure and does not flash 0 (plan g2.2)', async () => {
    const w = mountMetrics()
    await flushPromises()
    expect(w.find('[data-testid="status-metrics-tokens"]').text()).toContain('1.24M')

    platformStatus.mockRejectedValueOnce(new Error('network'))
    vi.advanceTimersByTime(PLATFORM_STATUS_POLL_MS)
    await flushPromises()
    expect(w.find('[data-testid="status-metrics-tokens"]').text()).toContain('1.24M')
    expect(w.find('[data-testid="status-metrics"]').attributes('data-stale')).toBe('true')
    w.unmount()
  })

  it('pauses polling while document is hidden (plan g2.2)', async () => {
    const w = mountMetrics()
    await flushPromises()
    const calls = platformStatus.mock.calls.length

    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    document.dispatchEvent(new Event('visibilitychange'))
    vi.advanceTimersByTime(PLATFORM_STATUS_POLL_MS * 3)
    await flushPromises()
    expect(platformStatus.mock.calls.length).toBe(calls)

    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(platformStatus.mock.calls.length).toBeGreaterThan(calls)
    w.unmount()
  })

  it('shows — for null token fields and 0 for true-zero counts', async () => {
    platformStatus.mockResolvedValue({
      cumulativeTokens: null,
      current5mBucketTokens: null,
      todayMaxCompleted5mTokens: null,
      runningCount: 0,
      queuedCount: 0,
      asOf: '2026-08-12T00:00:00Z',
      timezone: 'UTC',
    })
    const w = mountMetrics()
    await flushPromises()
    await nextTick()
    expect(w.find('[data-testid="status-metrics-tokens"]').text()).toContain('—')
    expect(w.find('[data-testid="status-metrics-rate"]').text()).toContain('—')
    expect(w.find('[data-testid="status-metrics-peak"]').text()).toContain('—')
    expect(w.find('[data-testid="status-metrics-running"]').text()).toContain('0')
    expect(w.find('[data-testid="status-metrics-queued"]').text()).toContain('0')
    w.unmount()
  })

  it('renders Token·RUN/Q summary under md (plan g2.4)', async () => {
    isMobile.value = true
    const w = mountMetrics()
    await flushPromises()
    expect(w.find('[data-testid="status-metrics-compact"]').exists()).toBe(true)
    expect(w.find('[data-testid="status-metrics-tokens"]').exists()).toBe(false)
    const text = w.find('[data-testid="status-metrics-compact"]').text()
    expect(text).toMatch(/1\.24M/)
    expect(text).toMatch(/3/)
    expect(text).toMatch(/5/)
    w.unmount()
  })
})

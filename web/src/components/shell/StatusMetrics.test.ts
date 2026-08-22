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
    const compact = w.find('[data-testid="status-metrics-compact"]')
    // g1.1: elevated strip + semibold values for sidebar readability
    expect(compact.classes()).toContain('bg-elevated')
    expect(compact.classes()).toContain('sm-compact')
    expect(compact.find('.sm-val').classes()).toContain('font-semibold')
    const text = compact.text()
    expect(text).toMatch(/1\.24M/)
    expect(text).toMatch(/3/)
    expect(text).toMatch(/5/)
    w.unmount()
  })

  it('desktop tips are single-line label: value with exact counts (plan g1.1/g1.2)', async () => {
    const w = mountMetrics()
    await flushPromises()
    const tips = {
      tokens: w.find('[data-testid="status-metrics-tokens"] .sm-tip').text(),
      rate: w.find('[data-testid="status-metrics-rate"] .sm-tip').text(),
      peak: w.find('[data-testid="status-metrics-peak"] .sm-tip').text(),
      running: w.find('[data-testid="status-metrics-running"] .sm-tip').text(),
      queued: w.find('[data-testid="status-metrics-queued"] .sm-tip').text(),
    }
    expect(tips.tokens).toMatch(/累计 Token:\s*1,240,582/)
    expect(tips.rate).toMatch(/当前 5 分钟速率:\s*4,812/)
    expect(tips.peak).toMatch(/今日 5 分钟峰值:\s*12,104/)
    expect(tips.running).toMatch(/执行中:\s*3/)
    expect(tips.queued).toMatch(/排队:\s*5/)
    for (const tip of Object.values(tips)) {
      expect(tip).not.toMatch(/完整值/)
      expect(tip).not.toMatch(/totalTokens/i)
      expect(tip).not.toContain('/5m')
      expect(tip).not.toMatch(/\d{2}:\d{2}/)
    }
    expect(w.find('[data-testid="status-metrics-tokens"] .sm-tip').classes()).not.toContain('min-w-[210px]')
    w.unmount()
  })

  it('compact tip is five label: value rows aligned with desktop (plan g1.3)', async () => {
    isMobile.value = true
    const w = mountMetrics()
    await flushPromises()
    const tip = w.find('[data-testid="status-metrics-compact"] .sm-tip')
    const lines = tip.findAll('div').map((d) => d.text())
    expect(lines).toHaveLength(5)
    expect(lines[0]).toMatch(/累计 Token:\s*1,240,582/)
    expect(lines[1]).toMatch(/当前 5 分钟速率:\s*4,812/)
    expect(lines[2]).toMatch(/今日 5 分钟峰值:\s*12,104/)
    expect(lines[3]).toMatch(/执行中:\s*3/)
    expect(lines[4]).toMatch(/排队:\s*5/)
    const tipText = tip.text()
    expect(tipText).not.toMatch(/完整值/)
    expect(tipText).not.toContain('/5m')
    expect(tipText).not.toMatch(/窄屏摘要|完整值|totalTokens/i)
    w.unmount()
  })

  it('sidebar compact variant teleports tip above trigger (g1.1)', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...shell } },
    })
    const clip = document.createElement('div')
    clip.style.overflow = 'hidden'
    clip.style.height = '120px'
    document.body.appendChild(clip)

    const w = mount(StatusMetrics, {
      props: { variant: 'compact' },
      global: { plugins: [i18n] },
      attachTo: clip,
    })
    await flushPromises()

    const trigger = w.find('[data-testid="status-metrics-compact"]')
    expect(trigger.find('.sm-tip').exists()).toBe(false)

    vi.spyOn(trigger.element as HTMLElement, 'getBoundingClientRect').mockReturnValue({
      top: 320,
      left: 24,
      right: 200,
      bottom: 352,
      width: 176,
      height: 32,
      x: 24,
      y: 320,
      toJSON: () => ({}),
    } as DOMRect)

    await trigger.trigger('click')
    await flushPromises()
    await nextTick()

    const tip = document.body.querySelector('[data-testid="status-metrics-compact-tip"]') as HTMLElement
    expect(tip).toBeTruthy()
    expect(tip.getAttribute('data-placement')).toBe('above')
    expect(tip.style.position).toBe('fixed')
    expect(tip.textContent).toMatch(/累计 Token:\s*1,240,582/)
    expect(tip.textContent).toMatch(/排队:\s*5/)
    expect(Number.parseInt(tip.style.top, 10)).toBeLessThan(320)

    w.unmount()
    clip.remove()
    document.body.innerHTML = ''
  })

  it('zero counts still show label: 0 in tip', async () => {
    platformStatus.mockResolvedValue({
      cumulativeTokens: 0,
      current5mBucketTokens: 0,
      todayMaxCompleted5mTokens: 0,
      runningCount: 0,
      queuedCount: 0,
      asOf: '2026-08-12T00:00:00Z',
      timezone: 'UTC',
    })
    const w = mountMetrics()
    await flushPromises()
    expect(w.find('[data-testid="status-metrics-running"] .sm-tip').text()).toMatch(/执行中:\s*0/)
    expect(w.find('[data-testid="status-metrics-queued"] .sm-tip').text()).toMatch(/排队:\s*0/)
    expect(w.find('[data-testid="status-metrics-tokens"] .sm-tip').text()).toMatch(/累计 Token:\s*0/)
    w.unmount()
  })
})

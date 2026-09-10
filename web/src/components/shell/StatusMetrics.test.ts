// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
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

function makeI18n() {
  return createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...shell } },
  })
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      {
        path: '/stats',
        name: 'stats',
        component: { template: '<div data-testid="token-analytics-page" />' },
      },
    ],
  })
}

async function mountMetrics(props?: { variant?: 'auto' | 'full' | 'compact' }) {
  const i18n = makeI18n()
  const router = makeRouter()
  await router.push('/')
  const w = mount(StatusMetrics, {
    props,
    global: { plugins: [i18n, router] },
    attachTo: document.body,
  })
  return { w, router }
}

describe('StatusMetrics', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    isMobile.value = false
    platformStatus.mockReset()
    platformStatus.mockResolvedValue({
      cumulativeTokens: 1240582,
      todayTokens: 4812,
      runningCount: 3,
      queuedCount: 5,
      asOf: '2026-08-12T06:07:00Z',
      timezone: 'Asia/Shanghai',
    })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('renders four desktop metrics with today tokens (plan g2.2)', async () => {
    const { w } = await mountMetrics()
    await flushPromises()
    expect(w.find('[data-testid="status-metrics"]').exists()).toBe(true)
    expect(w.find('[data-testid="status-metrics-tokens"]').text()).toContain('1.24M')
    expect(w.find('[data-testid="status-metrics-today"]').text()).toContain('4.8K')
    expect(w.find('[data-testid="status-metrics-today"]').text()).not.toContain('/5m')
    expect(w.find('[data-testid="status-metrics-rate"]').exists()).toBe(false)
    expect(w.find('[data-testid="status-metrics-peak"]').exists()).toBe(false)
    expect(w.find('[data-testid="status-metrics-running"]').text()).toContain('3')
    expect(w.find('[data-testid="status-metrics-queued"]').text()).toContain('5')
    w.unmount()
  })

  it('keeps lastSuccess on failure and does not flash 0 (plan g2.2)', async () => {
    const { w } = await mountMetrics()
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
    const { w } = await mountMetrics()
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
      todayTokens: null,
      runningCount: 0,
      queuedCount: 0,
      asOf: '2026-08-12T00:00:00Z',
      timezone: 'UTC',
    })
    const { w } = await mountMetrics()
    await flushPromises()
    await nextTick()
    expect(w.find('[data-testid="status-metrics-tokens"]').text()).toContain('—')
    expect(w.find('[data-testid="status-metrics-today"]').text()).toContain('—')
    expect(w.find('[data-testid="status-metrics-running"]').text()).toContain('0')
    expect(w.find('[data-testid="status-metrics-queued"]').text()).toContain('0')
    w.unmount()
  })

  it('renders Token·RUN/Q summary under md (plan g2.4)', async () => {
    isMobile.value = true
    const { w } = await mountMetrics()
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
    const { w } = await mountMetrics()
    await flushPromises()
    const tips = {
      tokens: w.find('[data-testid="status-metrics-tokens"] .sm-tip').text(),
      today: w.find('[data-testid="status-metrics-today"] .sm-tip').text(),
      running: w.find('[data-testid="status-metrics-running"] .sm-tip').text(),
      queued: w.find('[data-testid="status-metrics-queued"] .sm-tip').text(),
    }
    expect(tips.tokens).toMatch(/累计 Token:\s*1,240,582/)
    expect(tips.today).toMatch(/今日 Token:\s*4,812/)
    expect(w.find('[data-testid="status-metrics-today"]').attributes('aria-label')).toMatch(/今日 Token:\s*4,812/)
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

  it('compact tip is four label: value rows aligned with desktop (plan g2.3)', async () => {
    isMobile.value = true
    const { w } = await mountMetrics()
    await flushPromises()
    const tip = w.find('[data-testid="status-metrics-compact"] .sm-tip')
    const lines = tip.findAll('div').map((d) => d.text())
    expect(lines).toHaveLength(4)
    expect(lines[0]).toMatch(/累计 Token:\s*1,240,582/)
    expect(lines[1]).toMatch(/今日 Token:\s*4,812/)
    expect(lines[2]).toMatch(/执行中:\s*3/)
    expect(lines[3]).toMatch(/排队:\s*5/)
    const tipText = tip.text()
    expect(tipText).not.toMatch(/完整值/)
    expect(tipText).not.toContain('/5m')
    expect(tipText).not.toMatch(/5 分钟|速率|峰值/)
    expect(tipText).not.toMatch(/窄屏摘要|完整值|totalTokens/i)
    w.unmount()
  })

  it('sidebar compact variant teleports tip above trigger on hover (g1.1/g1.3)', async () => {
    const clip = document.createElement('div')
    clip.style.overflow = 'hidden'
    clip.style.height = '120px'
    document.body.appendChild(clip)

    const i18n = makeI18n()
    const router = makeRouter()
    await router.push('/')
    const w = mount(StatusMetrics, {
      props: { variant: 'compact' },
      global: { plugins: [i18n, router] },
      attachTo: clip,
    })
    await flushPromises()

    const trigger = w.find('[data-testid="status-metrics-compact"]')
    expect(trigger.find('.sm-tip').exists()).toBe(false)
    expect(trigger.attributes('aria-label')).toMatch(/进入统计/)

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

    await trigger.trigger('mouseenter')
    await flushPromises()
    await nextTick()

    const tip = document.body.querySelector('[data-testid="status-metrics-compact-tip"]') as HTMLElement
    expect(tip).toBeTruthy()
    expect(tip.getAttribute('data-placement')).toBe('above')
    expect(tip.style.position).toBe('fixed')
    expect(tip.textContent).toMatch(/累计 Token:\s*1,240,582/)
    expect(tip.textContent).toMatch(/排队:\s*5/)
    expect(Number.parseInt(tip.style.top, 10)).toBeLessThan(320)

    await trigger.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('stats')
    const afterClick = document.body.querySelector(
      '[data-testid="status-metrics-compact-tip"]',
    ) as HTMLElement | null
    expect(afterClick?.style.display === 'none' || afterClick == null).toBe(true)

    w.unmount()
    clip.remove()
    document.body.innerHTML = ''
  })

  it('click compact navigates to stats and closes teleport tip (plan g1.1)', async () => {
    const { w, router } = await mountMetrics({ variant: 'compact' })
    await flushPromises()
    const compact = w.find('[data-testid="status-metrics-compact"]')
    await compact.trigger('mouseenter')
    await flushPromises()
    expect(document.body.querySelector('[data-testid="status-metrics-compact-tip"]')).toBeTruthy()
    await compact.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('stats')
    expect(router.currentRoute.value.path).toBe('/stats')
    const tip = document.body.querySelector('[data-testid="status-metrics-compact-tip"]') as HTMLElement | null
    expect(tip == null || (tip as HTMLElement).style.display === 'none' || getComputedStyle(tip).display === 'none').toBe(true)
    w.unmount()
  })

  it('Enter/Space on compact navigates to stats (plan g1.1)', async () => {
    const { w, router } = await mountMetrics({ variant: 'compact' })
    await flushPromises()
    await w.find('[data-testid="status-metrics-compact"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('stats')
    w.unmount()

    const again = await mountMetrics({ variant: 'compact' })
    await flushPromises()
    await again.w.find('[data-testid="status-metrics-compact"]').trigger('keydown', { key: ' ' })
    await flushPromises()
    expect(again.router.currentRoute.value.name).toBe('stats')
    again.w.unmount()
  })

  it('desktop four items navigate to stats instead of pinning tip (plan g1.2)', async () => {
    const ids = [
      'status-metrics-tokens',
      'status-metrics-today',
      'status-metrics-running',
      'status-metrics-queued',
    ] as const
    for (const id of ids) {
      const { w, router } = await mountMetrics()
      await flushPromises()
      expect(w.find(`[data-testid="${id}"]`).attributes('aria-label')).toMatch(/进入统计/)
      await w.find(`[data-testid="${id}"]`).trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.name).toBe('stats')
      expect(w.find(`[data-testid="${id}"]`).classes()).not.toContain('tip-open')
      w.unmount()
    }
  })

  it('stays on stats when already there (plan g1.1)', async () => {
    const { w, router } = await mountMetrics()
    await router.push({ name: 'stats' })
    await flushPromises()
    await w.find('[data-testid="status-metrics-tokens"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('stats')
    expect(router.currentRoute.value.path).toBe('/stats')
    w.unmount()
  })

  it('zero counts still show label: 0 in tip', async () => {
    platformStatus.mockResolvedValue({
      cumulativeTokens: 0,
      todayTokens: 0,
      runningCount: 0,
      queuedCount: 0,
      asOf: '2026-08-12T00:00:00Z',
      timezone: 'UTC',
    })
    const { w } = await mountMetrics()
    await flushPromises()
    expect(w.find('[data-testid="status-metrics-running"] .sm-tip').text()).toMatch(/执行中:\s*0/)
    expect(w.find('[data-testid="status-metrics-queued"] .sm-tip').text()).toMatch(/排队:\s*0/)
    expect(w.find('[data-testid="status-metrics-tokens"] .sm-tip').text()).toMatch(/累计 Token:\s*0/)
    expect(w.find('[data-testid="status-metrics-today"] .sm-tip').text()).toMatch(/今日 Token:\s*0/)
    w.unmount()
  })

  it('uses A-set stroke icon paths (plan g2.4)', async () => {
    const { w } = await mountMetrics()
    await flushPromises()
    const html = w.html()
    expect(html).toContain('cx="12" cy="6.6" rx="7.2" ry="3.1"')
    expect(html).toContain('M4.8 6.6v4.7c0 1.7 3.2 3.1 7.2 3.1s7.2-1.4 7.2-3.1V6.6')
    expect(html).toContain('M8.2 3v4.2M15.8 3v4.2M3.4 10.2h17.2')
    expect(html).toContain('M10.3 8.7l5.4 3.3-5.4 3.3z')
    expect(html).toContain('M4 7.2h16M4 12h11.5M4 16.8h7')
    expect(html).not.toContain('M13 3L5 14h7l-1 7 8-11h-7l1-7z')
    expect(html).not.toContain('fill="currentColor"')
    expect(html).not.toContain('M5 7h14M5 12h14M5 17h10')
    w.unmount()
  })
})

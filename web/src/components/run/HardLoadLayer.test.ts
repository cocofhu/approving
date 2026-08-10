// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import commonEn from '@/locales/en/common.json'
import HardLoadLayer from './HardLoadLayer.vue'

function mountLayer(
  props: { stuckAfterMs?: number; stage?: string; overlay?: boolean } = {},
  locale: 'zh-CN' | 'en' = 'zh-CN',
) {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: { 'zh-CN': { ...common }, en: { ...commonEn } },
  })
  return mount(HardLoadLayer, {
    props: { stuckAfterMs: 10_000, ...props },
    global: { plugins: [i18n] },
  })
}

describe('HardLoadLayer', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows stage, elapsed, and heartbeat without stuck warning before threshold', () => {
    vi.useFakeTimers()
    const w = mountLayer({ stuckAfterMs: 10_000, stage: '加载中' })
    expect(w.get('[data-testid="hard-load-layer"]').attributes('aria-busy')).toBe('true')
    expect(w.find('[data-testid="hard-load-heartbeat"]').exists()).toBe(true)
    expect(w.get('[data-testid="hard-load-stage"]').text()).toBe('加载中')
    expect(w.get('[data-testid="hard-load-elapsed"]').text()).toMatch(/已用时 0s/)
    expect(w.find('[data-testid="hard-load-stuck"]').exists()).toBe(false)
    vi.advanceTimersByTime(9_000)
    expect(w.find('[data-testid="hard-load-stuck"]').exists()).toBe(false)
    w.unmount()
  })

  it('warns may-be-stuck after threshold without auto-failing', async () => {
    vi.useFakeTimers()
    const w = mountLayer({ stuckAfterMs: 10_000 })
    vi.advanceTimersByTime(10_000)
    await w.vm.$nextTick()
    expect(w.get('[data-testid="hard-load-stuck"]').text()).toContain('可能卡死')
    expect(w.get('[data-testid="hard-load-retry"]').text()).toBe('重试')
    expect(w.get('[data-testid="hard-load-layer"]').attributes('aria-busy')).toBe('true')
    w.unmount()
  })

  it('retry resets elapsed and emits retry', async () => {
    vi.useFakeTimers()
    const w = mountLayer({ stuckAfterMs: 1_000 })
    vi.advanceTimersByTime(1_000)
    await w.vm.$nextTick()
    expect(w.find('[data-testid="hard-load-stuck"]').exists()).toBe(true)
    await w.get('[data-testid="hard-load-retry"]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
    expect(w.find('[data-testid="hard-load-stuck"]').exists()).toBe(false)
    expect(w.get('[data-testid="hard-load-elapsed"]').text()).toMatch(/已用时 0s/)
    w.unmount()
  })

  it('renders English Demo-locked copy', async () => {
    vi.useFakeTimers()
    const w = mountLayer({ stuckAfterMs: 20_000, stage: 'Starting…' }, 'en')
    expect(w.get('[data-testid="hard-load-stage"]').text()).toBe('Starting…')
    expect(w.get('[data-testid="hard-load-elapsed"]').text()).toMatch(/Elapsed 0s/)
    vi.advanceTimersByTime(20_000)
    await w.vm.$nextTick()
    expect(w.get('[data-testid="hard-load-stuck"]').text()).toMatch(/May be stuck/i)
    w.unmount()
  })
})

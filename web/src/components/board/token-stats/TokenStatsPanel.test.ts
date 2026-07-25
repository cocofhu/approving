// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { ProjectTokenStats } from '@/lib/types'
import TokenStatsPanel from './TokenStatsPanel.vue'

const getProjectTokenStats = vi.fn()

vi.mock('@/lib/api', () => ({
  api: {
    getProjectTokenStats: (...args: unknown[]) => getProjectTokenStats(...args),
  },
}))

function sampleStats(partial: Partial<ProjectTokenStats> = {}): ProjectTokenStats {
  return {
    window: '30d',
    bucketWidth: 'day',
    timezone: 'Asia/Shanghai',
    empty: false,
    trend: [
      {
        bucket: '2026-07-24',
        total: 100,
        inputTokens: 40,
        outputTokens: 30,
        cacheReadTokens: 20,
        cacheWriteTokens: 10,
      },
      {
        bucket: '2026-07-25',
        total: 80,
        inputTokens: 30,
        outputTokens: 25,
        cacheReadTokens: 15,
        cacheWriteTokens: 10,
      },
    ],
    composition: {
      inputTokens: 70,
      outputTokens: 55,
      cacheReadTokens: 35,
      cacheWriteTokens: 20,
      total: 180,
    },
    workflows: [
      { workflowId: 'wf-a', name: 'approve-main', total: 120 },
      { workflowId: 'wf-b', name: 'doc-review', total: 40 },
      { name: 'other', total: 20, other: true },
    ],
    ...partial,
  }
}

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(TokenStatsPanel, {
    props: { projectId: 'proj-1' },
    global: {
      plugins: [i18n],
      stubs: {
        TokenTrendChart: { template: '<div data-testid="stub-trend" />' },
        TokenDonutChart: { template: '<div data-testid="stub-donut" />' },
        TokenWorkflowRank: { template: '<div data-testid="stub-rank" />' },
      },
    },
  })
}

describe('TokenStatsPanel', () => {
  beforeEach(() => {
    getProjectTokenStats.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('loads default 30d window and renders charts (g2.1/g2.2)', async () => {
    getProjectTokenStats.mockResolvedValue(sampleStats())
    const wrapper = mountPanel()
    expect(wrapper.find('[data-testid="token-stats-loading"]').exists()).toBe(true)
    await flushPromises()
    expect(getProjectTokenStats).toHaveBeenCalledWith(
      'proj-1',
      expect.objectContaining({ window: '30d' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(wrapper.find('[data-testid="token-stats-charts"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-stats-window-badge"]').text()).toContain('近 30 天')
    expect(wrapper.find('[data-testid="token-stats-window-30d"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('shows empty state when API reports empty (g2.5 null≠0)', async () => {
    getProjectTokenStats.mockResolvedValue(sampleStats({ empty: true, trend: [], workflows: [] }))
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="token-stats-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-stats-charts"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('未上报不会显示为 0')
    wrapper.unmount()
  })

  it('shows unified failure + retry and clears stale data on window switch (g2.1/g2.5)', async () => {
    getProjectTokenStats.mockResolvedValueOnce(sampleStats())
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="token-stats-charts"]').exists()).toBe(true)

    let resolveNext: (v: ProjectTokenStats) => void = () => {}
    getProjectTokenStats.mockImplementationOnce(
      () =>
        new Promise<ProjectTokenStats>((resolve) => {
          resolveNext = resolve
        }),
    )
    await wrapper.find('[data-testid="token-stats-window-7d"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="token-stats-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-stats-charts"]').exists()).toBe(false)

    resolveNext(sampleStats({ window: '7d' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="token-stats-charts"]').exists()).toBe(true)

    getProjectTokenStats.mockRejectedValueOnce(new Error('network'))
    await wrapper.find('[data-testid="token-stats-window-all"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="token-stats-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-stats-retry"]').exists()).toBe(true)

    getProjectTokenStats.mockResolvedValueOnce(sampleStats({ window: 'all', bucketWidth: 'week' }))
    await wrapper.find('[data-testid="token-stats-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="token-stats-charts"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

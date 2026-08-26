// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import TokenAnalyticsView from './TokenAnalyticsView.vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import nav from '@/locales/zh-CN/nav.json'
import route from '@/locales/zh-CN/route.json'

vi.mock('@/lib/api/api', () => ({
  api: {
    getGlobalTokenStats: vi.fn(),
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('vue-echarts', () => ({
  default: {
    name: 'VChart',
    template: '<div data-testid="mock-vchart"><canvas /></div>',
    props: ['option'],
  },
}))

vi.mock('@/components/charts/echartsSetup', () => ({
  registerECharts: () => {},
}))

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { ...common, ...pages, ...nav, ...route } },
})

import { api } from '@/lib/api/api'

const sampleData = {
  window: '30d',
  bucketWidth: 'day',
  timezone: 'Asia/Shanghai',
  empty: false,
  kpi: {
    total: 5000,
    deltaPct: 12.5,
    inputTokens: 3000,
    outputTokens: 1500,
    cacheReadTokens: 400,
    cacheWriteTokens: 100,
    workflowTotal: 4200,
    pmTotal: 800,
    projectCount: 2,
    runCount: 5,
    modelCount: 2,
  },
  trend: [{ bucket: '2026-07-01', total: 100, workflowTotal: 80, pmTotal: 20, inputTokens: 40, outputTokens: 30, cacheReadTokens: 20, cacheWriteTokens: 10 }],
  prevTrend: [{ bucket: '2026-06-01', total: 80, workflowTotal: 60, pmTotal: 20, inputTokens: 30, outputTokens: 25, cacheReadTokens: 15, cacheWriteTokens: 10 }],
  composition: { inputTokens: 3000, outputTokens: 1500, cacheReadTokens: 400, cacheWriteTokens: 100, total: 5000 },
  projects: [{ projectId: 'p1', name: 'Approving', total: 3000, inputTokens: 1800, outputTokens: 900 }],
  modelRanking: [{ modelKey: 'sonnet', name: 'Sonnet', total: 4000 }],
  nodeTypes: [{ name: 'agent', total: 4000 }],
  workflows: [{ workflowId: 'w1', name: 'main', total: 3000, kind: 'workflow' as const }],
  heatmap: { rows: ['Sonnet'], cols: ['Approving'], grid: [[3000]] },
  topRuns: [{ runId: 'r1', title: 'Run 1', projectId: 'p1', projectName: 'Approving', workflowName: 'main', modelKey: 'sonnet', modelName: 'Sonnet', total: 500 }],
  projectTrends: [{ key: 'p1', name: 'Approving', trend: [{ bucket: '2026-07-01', total: 100, workflowTotal: 80, pmTotal: 20, inputTokens: 40, outputTokens: 30, cacheReadTokens: 20, cacheWriteTokens: 10 }] }],
  modelTrends: [{ key: 'sonnet', name: 'Sonnet', trend: [{ bucket: '2026-07-01', total: 100, workflowTotal: 80, pmTotal: 20, inputTokens: 40, outputTokens: 30, cacheReadTokens: 20, cacheWriteTokens: 10 }] }],
  filterOptions: { projects: [{ key: 'p1', name: 'Approving' }], models: [{ key: 'sonnet', name: 'Sonnet' }] },
}

describe('TokenAnalyticsView', () => {
  beforeEach(() => {
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue(sampleData)
  })

  it('loads global stats and shows chart sections', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-page"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-analytics-lines"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-analytics-pies"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('全局 Token 分析台')
    wrapper.unmount()
  })

  it('shows empty state when API returns empty', async () => {
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue({ ...sampleData, empty: true, kpi: { ...sampleData.kpi, total: 0 } })
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

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

const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
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
import { displayRunTitle } from '@/lib/run/runTitle'

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
  projects: [{ projectId: 'p1', name: 'Approving', total: 3000, inputTokens: 1800, outputTokens: 900, cacheReadTokens: 200, cacheWriteTokens: 100 }],
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
    pushMock.mockClear()
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue(sampleData)
  })

  it('loads global stats with h-full root and three KPI cards', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-page"]').classes()).toContain('h-full')
    expect(wrapper.find('[data-testid="token-analytics-kpis"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-analytics-kpi-total"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-analytics-kpi-merge"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-analytics-kpi-scope"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="token-analytics-kpi-total"], [data-testid="token-analytics-kpi-merge"], [data-testid="token-analytics-kpi-scope"]')).toHaveLength(3)
    wrapper.unmount()
  })

  it('uses plain-language title and hides section navigation', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.text()).toContain('用量统计')
    expect(wrapper.text()).not.toContain('全局 Token 分析台')
    expect(wrapper.text()).not.toContain('较上一窗')
    expect(wrapper.text()).not.toContain('vs previous window')
    expect(wrapper.find('[data-testid="token-analytics-section-nav-mobile"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="token-analytics-section-nav"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('统计导航')
    expect(wrapper.text()).not.toContain('四分量')
    expect(wrapper.text()).not.toContain('多折线')
    wrapper.unmount()
  })

  it('shows default input/output rows without cache labels or slash pairs', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const merge = wrapper.find('[data-testid="token-analytics-kpi-merge"]')
    const faceText = Array.from(merge.element.children)
      .filter((el) => el.classList.contains('mt-2.5'))
      .map((el) => el.textContent || '')
      .join('')
    expect(faceText).toContain('输入')
    expect(faceText).toContain('输出')
    expect(faceText).not.toContain('缓存读')
    expect(faceText).not.toContain('缓存写')
    expect(faceText).not.toMatch(/\d[\d,.]*[KMB]?\s*\/\s*\d/)
    expect(wrapper.find('[data-testid="token-analytics-kpi-detail"]').classes()).toContain('hidden')
    wrapper.unmount()
  })

  it('shows four-part KPI detail on hover and focus', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-kpi-detail"]').exists()).toBe(true)
    const merge = wrapper.find('[data-testid="token-analytics-kpi-merge"]')
    await merge.trigger('mouseenter')
    const detail = wrapper.find('[data-testid="token-analytics-kpi-detail"]')
    expect(detail.text()).toContain('3,000')
    expect(detail.text()).toContain('1,500')
    expect(detail.text()).toContain('400')
    expect(detail.text()).toContain('100')
    expect(detail.text()).toContain('缓存读')
    expect(detail.text()).toContain('缓存写')
    await merge.trigger('focus')
    expect(detail.text()).toContain('缓存读')
    wrapper.unmount()
  })

  it('uses overflow-visible plot areas so tooltips are not clipped', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-plot-lines"]').classes()).toContain('overflow-visible')
    expect(wrapper.find('[data-testid="token-analytics-plot-bars"]').classes()).toContain('overflow-visible')
    expect(wrapper.find('[data-testid="token-analytics-plot-area"]').classes()).toContain('overflow-visible')
    expect(wrapper.find('[data-testid="token-analytics-plot-heat"]').classes()).toContain('overflow-visible')
    wrapper.unmount()
  })

  it('pie charts use right-side legend, visible labels, and compact tooltip', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const charts = wrapper.findAllComponents({ name: 'VChart' })
    expect(charts.length).toBeGreaterThan(0)
    const pieChart = charts.find((c) => {
      const opt = c.props('option') as { series?: { type?: string }[] }
      return opt?.series?.[0]?.type === 'pie'
    })
    expect(pieChart).toBeTruthy()
    const option = pieChart!.props('option') as {
      legend?: { orient?: string; right?: number; bottom?: number }
      tooltip?: { formatter?: unknown; appendToBody?: boolean }
      series?: { label?: { show?: boolean }; center?: string[]; radius?: string | string[] }[]
    }
    expect(option.legend?.orient).toBe('vertical')
    expect(option.legend?.right).toBe(0)
    expect(option.legend?.bottom).toBeUndefined()
    expect(option.series?.[0]?.label?.show).toBe(true)
    expect(option.series?.[0]?.center?.[0]).toBe('38%')
    expect(typeof option.tooltip?.formatter).toBe('function')
    expect(option.tooltip?.appendToBody).toBe(true)
    expect((option.tooltip as { borderRadius?: number })?.borderRadius).toBe(0)
    expect(JSON.stringify(option.tooltip)).not.toContain('#1a1d23')
    wrapper.unmount()
  })

  it('axis charts use compact tooltip formatters and append tooltips to body', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const charts = wrapper.findAllComponents({ name: 'VChart' })
    const lineChart = charts.find((c) => {
      const opt = c.props('option') as { series?: { type?: string }[] }
      return opt?.series?.[0]?.type === 'line' && opt?.series?.length === 2
    })
    expect(lineChart).toBeTruthy()
    const option = lineChart!.props('option') as {
      tooltip?: { valueFormatter?: (v: number) => string; appendToBody?: boolean; confine?: boolean }
      yAxis?: { axisLabel?: { formatter?: (v: number) => string } }
    }
    expect(option.tooltip?.appendToBody).toBe(true)
    expect(option.tooltip?.confine).toBe(false)
    expect(option.tooltip?.valueFormatter?.(2_080_982_825)).toBe('2.08B')
    expect(option.yAxis?.axisLabel?.formatter?.(2_500_000_000)).toBe('2.5B')
    wrapper.unmount()
  })

  it('does not render pie caption text under chart titles', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const text = wrapper.text()
    expect(text).not.toMatch(/agent \d+%/)
    expect(text).not.toMatch(/main \d+%/)
    expect(wrapper.findAll('.text-\\[11px\\].text-txt3').filter((el) => el.text().includes('% · '))).toHaveLength(0)
    wrapper.unmount()
  })

  it('shows empty state when API returns empty', async () => {
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue({ ...sampleData, empty: true, kpi: { ...sampleData.kpi, total: 0 } })
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('navigates to project board when clicking project name', async () => {
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const projectBtn = wrapper.findAll('button.text-accent-2').find((b) => b.text() === 'Approving')
    expect(projectBtn).toBeTruthy()
    await projectBtn!.trigger('click')
    expect(pushMock).toHaveBeenCalledWith({ path: '/projects/p1', query: { tab: 'board' } })
    wrapper.unmount()
  })

  it('truncates long multiline run titles to a single line in top runs table', async () => {
    // plan coverage: g2.1 — 60-char cap, single line, tooltip for overflow
    const longTitle =
      '5. 迁移编号须严格递增，不得重用历史编号；6. 所有新增表结构变更必须配套 SQLite migration，禁止只改 Go struct；7. 关键路径必须有可重现的 Go tests。\n\n三、接口与安全\n对外 HTTP 接口保持向后兼容；敏感字段不得出现在日志。\n\n四、验收测试\n须覆盖主流程与边界，PR 不得带未测试的 Sendable 变更。'
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue({
      ...sampleData,
      topRuns: [{ ...sampleData.topRuns[0], title: longTitle }],
    })
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const runsTable = wrapper.find('[data-testid="token-analytics-runs-table"]')
    const runBtn = runsTable.find('tbody button.text-accent-2')
    const displayed = runBtn.text()
    expect(displayed.endsWith('…')).toBe(true)
    expect(displayed.length).toBeLessThanOrEqual(61)
    expect(displayed).not.toContain('\n')
    expect(displayed).not.toContain('三、接口与安全')
    expect(runBtn.classes()).toContain('truncate')
    const tooltip = displayRunTitle(longTitle).replace(/\s+/g, ' ').trim()
    expect(runBtn.attributes('title')).toBe(tooltip)
    expect(tooltip.length).toBeGreaterThan(60)
    wrapper.unmount()
  })

  it('shows short run titles without ellipsis in top runs table', async () => {
    // plan coverage: g2.1 — short titles keep full text, no tooltip
    const shortTitle = '我要讲解一个技术 或者一个产品 目前模板太少了'
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue({
      ...sampleData,
      topRuns: [{ ...sampleData.topRuns[0], title: shortTitle }],
    })
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const runsTable = wrapper.find('[data-testid="token-analytics-runs-table"]')
    const runBtn = runsTable.find('tbody button.text-accent-2')
    expect(runBtn.text()).toBe(shortTitle)
    expect(runBtn.attributes('title')).toBeUndefined()
    wrapper.unmount()
  })

  it('navigates to run detail when clicking truncated run title', async () => {
    // plan coverage: g2.1 — click truncated title still goes to /runs/:id
    const longTitle = 'A'.repeat(80)
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue({
      ...sampleData,
      topRuns: [{ ...sampleData.topRuns[0], runId: 'r-long', title: longTitle }],
    })
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const runsTable = wrapper.find('[data-testid="token-analytics-runs-table"]')
    await runsTable.find('tbody button.text-accent-2').trigger('click')
    expect(pushMock).toHaveBeenCalledWith('/runs/r-long')
    wrapper.unmount()
  })

  function firePointer(
    el: Element,
    type: 'pointerdown' | 'pointermove' | 'pointerup' | 'pointercancel',
    clientX: number,
  ) {
    el.dispatchEvent(
      new PointerEvent(type, {
        bubbles: true,
        cancelable: true,
        clientX,
        pointerId: 1,
      }),
    )
  }

  it('renders header sashes with i18n aria-label even when topRuns is empty', async () => {
    // plan coverage: g1.1 / f5 — empty topRuns still shows four draggable headers
    vi.mocked(api.getGlobalTokenStats).mockResolvedValue({ ...sampleData, topRuns: [] })
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const runsTable = wrapper.find('[data-testid="token-analytics-runs-table"]')
    expect(runsTable.text()).toContain('运行')
    expect(runsTable.text()).toContain('项目')
    expect(runsTable.text()).toContain('模型')
    expect(runsTable.text()).toContain('总量')
    for (const key of ['run', 'project', 'model', 'total'] as const) {
      const sash = runsTable.find(`[data-testid="token-analytics-runs-col-sash-${key}"]`)
      expect(sash.exists()).toBe(true)
      expect(sash.classes()).toContain('cursor-col-resize')
      expect(sash.attributes('role')).toBe('separator')
      expect(sash.attributes('aria-label')).toMatch(/拖动调整/)
    }
    wrapper.unmount()
  })

  it('dragging a header sash changes that column width', async () => {
    // plan coverage: g1.2 / g2.2 — pointer drag updates col style width
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const sash = wrapper.find('[data-testid="token-analytics-runs-col-sash-run"]').element
    const col = wrapper.find('[data-testid="token-analytics-runs-col-run"]')
    expect(col.attributes('style')).toContain('220px')
    firePointer(sash, 'pointerdown', 100)
    firePointer(sash, 'pointermove', 160)
    firePointer(sash, 'pointerup', 160)
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-runs-col-run"]').attributes('style')).toContain('280px')
    wrapper.unmount()
  })

  it('resizes each of the four runs-table columns independently', async () => {
    // plan coverage: g1.2 — run/project/model/total each have their own pixel width
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const drags: Array<[string, number, number]> = [
      ['run', 100, 150],
      ['project', 100, 130],
      ['model', 100, 90],
      ['total', 100, 140],
    ]
    for (const [key, start, end] of drags) {
      const sash = wrapper.find(`[data-testid="token-analytics-runs-col-sash-${key}"]`).element
      firePointer(sash, 'pointerdown', start)
      firePointer(sash, 'pointermove', end)
      firePointer(sash, 'pointerup', end)
    }
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-runs-col-run"]').attributes('style')).toContain('270px')
    expect(wrapper.find('[data-testid="token-analytics-runs-col-project"]').attributes('style')).toContain('170px')
    expect(wrapper.find('[data-testid="token-analytics-runs-col-model"]').attributes('style')).toContain('130px')
    expect(wrapper.find('[data-testid="token-analytics-runs-col-total"]').attributes('style')).toContain('128px')
    wrapper.unmount()
  })

  it('clamps column width at the minimum when dragging inward', async () => {
    // plan coverage: g1.2 / g2.2 — drag below min width is clamped
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const sash = wrapper.find('[data-testid="token-analytics-runs-col-sash-run"]').element
    firePointer(sash, 'pointerdown', 400)
    firePointer(sash, 'pointermove', -4000)
    firePointer(sash, 'pointerup', -4000)
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analytics-runs-col-run"]').attributes('style')).toContain('80px')
    wrapper.unmount()
  })

  it('dragging a column sash does not navigate to the run', async () => {
    // plan coverage: g1.3 / g2.2 — sash pointer drag must not call router.push
    const wrapper = mount(TokenAnalyticsView, { global: { plugins: [i18n] } })
    await flushPromises()
    const sash = wrapper.find('[data-testid="token-analytics-runs-col-sash-run"]').element
    firePointer(sash, 'pointerdown', 100)
    firePointer(sash, 'pointermove', 140)
    firePointer(sash, 'pointerup', 140)
    await flushPromises()
    expect(pushMock).not.toHaveBeenCalled()
    const runsTable = wrapper.find('[data-testid="token-analytics-runs-table"]')
    await runsTable.find('tbody button.text-accent-2').trigger('click')
    expect(pushMock).toHaveBeenCalledWith('/runs/r1')
    wrapper.unmount()
  })
})

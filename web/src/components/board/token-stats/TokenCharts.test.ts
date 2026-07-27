// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenTrendChart from './TokenTrendChart.vue'
import TokenDonutChart from './TokenDonutChart.vue'
import TokenWorkflowRank from './TokenWorkflowRank.vue'
import { TOKEN_SOURCE_COLORS } from './tokenStatsShared'

const i18n = () =>
  createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })

function sampleTrend(n: number) {
  return Array.from({ length: n }, (_, i) => {
    const day = String(i + 1).padStart(2, '0')
    const total = i === n - 1 ? 1_500_000 : i * 100
    const pm = Math.floor(total * 0.2)
    return {
      bucket: `2026-07-${day}`,
      total,
      workflowTotal: total - pm,
      pmTotal: pm,
      inputTokens: 40,
      outputTokens: 30,
      cacheReadTokens: 20,
      cacheWriteTokens: 10,
    }
  })
}

describe('Token charts (g2.3/g2.4)', () => {
  it('renders trend chart.js canvas from buckets including a zero day (g2.1)', () => {
    const wrapper = mount(TokenTrendChart, {
      props: {
        bucketWidth: 'day',
        trend: [
          {
            bucket: '2026-07-24',
            total: 100,
            workflowTotal: 70,
            pmTotal: 30,
            inputTokens: 40,
            outputTokens: 30,
            cacheReadTokens: 20,
            cacheWriteTokens: 10,
          },
          {
            bucket: '2026-07-25',
            total: 0,
            workflowTotal: 0,
            pmTotal: 0,
            inputTokens: 0,
            outputTokens: 0,
            cacheReadTokens: 0,
            cacheWriteTokens: 0,
          },
        ],
      },
      global: { plugins: [i18n()] },
    })
    expect(wrapper.find('[data-testid="token-trend-wrap"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-trend-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-trend-legend"]').text()).toContain('workflow')
    expect(wrapper.find('[data-testid="token-trend-legend"]').text()).toContain('pm')
    expect(wrapper.find('canvas').exists()).toBe(true)
    // Chart.js Canvas path — no legacy non-uniform SVG stretch host
    expect(wrapper.find('[data-testid="token-trend-svg"]').exists()).toBe(false)
    expect(wrapper.html()).not.toMatch(/preserveAspectRatio\s*=\s*["']none["']/)
    const exposed = wrapper.vm as unknown as {
      chartData: { datasets: { label: string; data: number[] }[] }
    }
    expect(exposed.chartData.datasets).toHaveLength(2)
    expect(exposed.chartData.datasets.map((d) => d.label)).toEqual(['workflow', 'pm'])
    wrapper.unmount()
  })

  it('keeps ~200px wrap height and rejects preserveAspectRatio=none stretch (g2.2)', () => {
    const wrapper = mount(TokenTrendChart, {
      props: {
        bucketWidth: 'day',
        trend: sampleTrend(30),
      },
      global: { plugins: [i18n()] },
      attachTo: document.body,
    })
    const wrap = wrapper.find('[data-testid="token-trend-wrap"]')
    expect(wrap.exists()).toBe(true)
    expect((wrap.element as HTMLElement).style.height).toBe('200px')
    expect(wrapper.find('svg[preserveAspectRatio="none"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('preserveAspectRatio="none"')
    // Wide container must still host canvas (aspect handled by Chart.js, not SVG none)
    ;(wrap.element as HTMLElement).style.width = '1280px'
    expect(wrapper.find('[data-testid="token-trend-chart"] canvas').exists()).toBe(true)
    wrapper.unmount()
  })

  it('trend wrap uses min-w-0 + overflow-x-clip to prevent Chart.js flex overflow (g3.1)', () => {
    const wrapper = mount(TokenTrendChart, {
      props: {
        bucketWidth: 'day',
        trend: sampleTrend(7),
      },
      global: { plugins: [i18n()] },
    })
    const wrap = wrapper.find('[data-testid="token-trend-wrap"]')
    expect(wrap.classes()).toEqual(expect.arrayContaining(['min-w-0', 'overflow-x-clip', 'w-full']))
    expect((wrap.element as HTMLElement).style.height).toBe('200px')
    wrapper.unmount()
  })

  it('maps 30-day labels with tick thinning options (maxTicksLimit≈8) (g2.2)', () => {
    const wrapper = mount(TokenTrendChart, {
      props: {
        bucketWidth: 'day',
        trend: sampleTrend(30),
      },
      global: { plugins: [i18n()] },
    })
    const exposed = wrapper.vm as unknown as {
      chartOptions: {
        maintainAspectRatio?: boolean
        scales?: { x?: { ticks?: { maxTicksLimit?: number; autoSkip?: boolean } } }
        plugins?: { tooltip?: { enabled?: boolean; external?: unknown } }
      }
      chartData: { labels: string[]; datasets: { data: number[] }[] }
    }
    const opts = exposed.chartOptions
    expect(opts.maintainAspectRatio).toBe(false)
    expect(opts.scales?.x?.ticks?.maxTicksLimit).toBe(8)
    expect(opts.scales?.x?.ticks?.autoSkip).toBe(true)
    expect(opts.plugins?.tooltip?.enabled).toBe(false)
    expect(typeof opts.plugins?.tooltip?.external).toBe('function')
    expect(exposed.chartData.labels).toHaveLength(30)
    expect(exposed.chartData.datasets[0]?.data).toHaveLength(30)
    wrapper.unmount()
  })

  it('renders donut ring with legend and center total', () => {
    const wrapper = mount(TokenDonutChart, {
      props: {
        composition: {
          inputTokens: 70,
          outputTokens: 55,
          cacheReadTokens: 35,
          cacheWriteTokens: 20,
          total: 180,
        },
      },
      global: { plugins: [i18n()] },
    })
    expect(wrapper.find('[data-testid="token-donut-svg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('input')
    expect(wrapper.text()).toContain('总量')
    wrapper.unmount()
  })

  it('narrow donut stacks ring above legend and shrinks ring (~120px) (g4.1/g4.2)', () => {
    const wrapper = mount(TokenDonutChart, {
      props: {
        composition: {
          inputTokens: 70,
          outputTokens: 55,
          cacheReadTokens: 35,
          cacheWriteTokens: 20,
          total: 180,
        },
      },
      global: { plugins: [i18n()] },
    })
    const row = wrapper.find('[data-testid="token-donut-row"]')
    expect(row.classes()).toEqual(expect.arrayContaining(['flex-col', 'sm:flex-row']))
    const svg = wrapper.find('[data-testid="token-donut-svg"]')
    expect(svg.classes()).toEqual(expect.arrayContaining(['h-[120px]', 'w-[120px]', 'sm:h-[150px]', 'sm:w-[150px]']))
    const legend = wrapper.find('[data-testid="token-donut-legend"]')
    expect(legend.classes()).toEqual(expect.arrayContaining(['w-full']))
    wrapper.unmount()
  })

  it('renders consumption rank with continuous numeric badges for workflow+PM (no 12PM34)', () => {
    const wrapper = mount(TokenWorkflowRank, {
      props: {
        // Fixed API order: Top workflows → PM → other
        workflows: [
          { workflowId: 'a', name: 'approve-main', total: 1_020_000, kind: 'workflow' },
          { workflowId: 'b', name: 'doc-review', total: 40_000, kind: 'workflow' },
          { name: 'PM', total: 80_000, kind: 'pm' },
          { name: 'other', total: 20_000, other: true, kind: 'other' },
        ],
      },
      global: { plugins: [i18n()] },
    })
    const items = wrapper.findAll('li')
    expect(items).toHaveLength(4)
    expect(wrapper.find('.token-rank-pm').exists()).toBe(true)
    expect(wrapper.find('.token-rank-other').exists()).toBe(true)
    expect(wrapper.find('[data-kind="pm"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('PM')
    expect(wrapper.text()).toContain('其他')

    // Continuous numeric badges for workflow+PM; other stays "·" (Demo: 1→2→3→·)
    const pmRow = wrapper.find('[data-kind="pm"]')
    const badges = wrapper.findAll('li').map((li) => li.find('span').text())
    expect(badges).toEqual(['1', '2', '3', '·'])
    expect(pmRow.find('span').text()).toBe('3')
    // PM badge style matches workflow numeric badge (accent for rank ≤3); no special w-auto/min-w
    const pmBadge = pmRow.find('span')
    expect(pmBadge.classes()).toEqual(
      expect.arrayContaining(['w-[22px]', 'bg-accent-dim', 'text-accent-2']),
    )
    expect(pmBadge.classes()).not.toEqual(expect.arrayContaining(['w-auto', 'min-w-[22px]', 'px-1.5']))
    const wfBadge = items[0]!.find('span')
    expect(wfBadge.classes()).toEqual(pmBadge.classes())

    // Same purple bar as workflow — no amber (#f59e0b / #d97706 / #fbbf24)
    const pmHtml = pmRow.html()
    expect(pmHtml).toContain('#6d5cff')
    expect(pmHtml).toContain('#9b8cff')
    expect(pmHtml).not.toMatch(/#f59e0b|#d97706|#fbbf24|245,\s*158,\s*11/)

    // Compact K/M main values remain visible on the right
    const values = wrapper.findAll('[data-testid="token-rank-value"]').map((v) => v.text())
    expect(values).toEqual(['1.02M', '40K', '80K', '20K'])
    expect(wrapper.find('[data-testid="token-rank-value"]').attributes('title')).toBeTruthy()
    wrapper.unmount()
  })

  it('Demo sample: 4 workflows + PM + other → badges 1→2→3→4→5→· (g3.3)', () => {
    const wrapper = mount(TokenWorkflowRank, {
      props: {
        workflows: [
          { workflowId: '1', name: '自我迭代', total: 1200, kind: 'workflow' },
          { workflowId: '2', name: '自我迭代·轻量', total: 240, kind: 'workflow' },
          { workflowId: '3', name: '调研', total: 140, kind: 'workflow' },
          { workflowId: '4', name: '产品宣传文章', total: 70, kind: 'workflow' },
          { name: 'PM', total: 180, kind: 'pm' },
          { name: 'other', total: 55, other: true, kind: 'other' },
        ],
      },
      global: { plugins: [i18n()] },
    })
    const badges = wrapper.findAll('li').map((li) => li.find('span').text())
    expect(badges).toEqual(['1', '2', '3', '4', '5', '·'])
    expect(wrapper.find('[data-kind="pm"]').find('span').text()).toBe('5')
    // rank 5 uses dim elevated style same as a workflow at #5 would
    expect(wrapper.find('[data-kind="pm"]').find('span').classes()).toEqual(
      expect.arrayContaining(['w-[22px]', 'bg-elevated', 'text-txt3']),
    )
    wrapper.unmount()
  })

  it('hides PM row when absent; no-other still continuous (g3.3)', () => {
    const noPm = mount(TokenWorkflowRank, {
      props: {
        workflows: [
          { workflowId: 'a', name: 'approve-main', total: 100, kind: 'workflow' },
          { name: 'other', total: 20, other: true, kind: 'other' },
        ],
      },
      global: { plugins: [i18n()] },
    })
    expect(noPm.find('[data-kind="pm"]').exists()).toBe(false)
    expect(noPm.findAll('li').map((li) => li.find('span').text())).toEqual(['1', '·'])
    noPm.unmount()

    const noOther = mount(TokenWorkflowRank, {
      props: {
        workflows: [
          { workflowId: 'a', name: 'approve-main', total: 100, kind: 'workflow' },
          { name: 'PM', total: 50, kind: 'pm' },
        ],
      },
      global: { plugins: [i18n()] },
    })
    expect(noOther.find('[data-kind="other"]').exists()).toBe(false)
    expect(noOther.findAll('li').map((li) => li.find('span').text())).toEqual(['1', '2'])
    noOther.unmount()
  })

  it('trend PM dataset shares workflow purple and uses borderDash (g2.2/g3.1)', () => {
    expect(TOKEN_SOURCE_COLORS.pm).toBe(TOKEN_SOURCE_COLORS.workflow)
    expect(TOKEN_SOURCE_COLORS.pm).toBe('#6d5cff')
    expect(TOKEN_SOURCE_COLORS.pm).not.toBe('#f59e0b')

    const wrapper = mount(TokenTrendChart, {
      props: {
        bucketWidth: 'day',
        trend: sampleTrend(7),
      },
      global: { plugins: [i18n()] },
    })
    const exposed = wrapper.vm as unknown as {
      chartData: {
        datasets: {
          label: string
          borderColor: string
          backgroundColor: string
          borderDash?: number[]
        }[]
      }
    }
    const pmDs = exposed.chartData.datasets.find((d) => d.label === 'pm')
    const wfDs = exposed.chartData.datasets.find((d) => d.label === 'workflow')
    expect(pmDs).toBeTruthy()
    expect(wfDs).toBeTruthy()
    expect(pmDs!.borderColor).toBe(wfDs!.borderColor)
    expect(pmDs!.borderColor).toBe('#6d5cff')
    expect(pmDs!.borderDash).toEqual([5, 4])
    expect(wfDs!.borderDash).toBeUndefined()
    expect(String(pmDs!.backgroundColor)).not.toMatch(/245,\s*158,\s*11/)
    wrapper.unmount()
  })
})

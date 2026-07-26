// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenTrendChart from './TokenTrendChart.vue'
import TokenDonutChart from './TokenDonutChart.vue'
import TokenWorkflowRank from './TokenWorkflowRank.vue'

const i18n = () =>
  createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })

function sampleTrend(n: number) {
  return Array.from({ length: n }, (_, i) => {
    const day = String(i + 1).padStart(2, '0')
    return {
      bucket: `2026-07-${day}`,
      total: i === n - 1 ? 1_500_000 : i * 100,
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
            inputTokens: 40,
            outputTokens: 30,
            cacheReadTokens: 20,
            cacheWriteTokens: 10,
          },
          {
            bucket: '2026-07-25',
            total: 0,
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
    expect(wrapper.find('canvas').exists()).toBe(true)
    // Chart.js Canvas path — no legacy non-uniform SVG stretch host
    expect(wrapper.find('[data-testid="token-trend-svg"]').exists()).toBe(false)
    expect(wrapper.html()).not.toMatch(/preserveAspectRatio\s*=\s*["']none["']/)
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

  it('renders Top10 rank bars and styles other distinctly without padding placeholders', () => {
    const wrapper = mount(TokenWorkflowRank, {
      props: {
        workflows: [
          { workflowId: 'a', name: 'approve-main', total: 100 },
          { workflowId: 'b', name: 'doc-review', total: 40 },
          { name: 'other', total: 20, other: true },
        ],
      },
      global: { plugins: [i18n()] },
    })
    const items = wrapper.findAll('li')
    expect(items).toHaveLength(3)
    expect(wrapper.find('.token-rank-other').exists()).toBe(true)
    expect(wrapper.text()).toContain('其他')
    wrapper.unmount()
  })
})

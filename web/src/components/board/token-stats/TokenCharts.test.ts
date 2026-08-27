// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenTrendChart from './TokenTrendChart.vue'
import TokenDonutChart from './TokenDonutChart.vue'
import TokenWorkflowRank from './TokenWorkflowRank.vue'
import { TOKEN_SOURCE_COLORS } from './tokenStatsShared'

vi.mock('vue-echarts', () => ({
  default: {
    name: 'VChart',
    template: '<div data-testid="mock-vchart" class="h-full w-full"><canvas /></div>',
    props: ['option'],
    emits: ['mousemove', 'globalout', 'click'],
  },
}))

vi.mock('@/components/charts/echartsSetup', () => ({
  registerECharts: () => {},
}))

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
  it('renders ECharts trend from buckets including a zero day (g2.1)', () => {
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
    const legend = wrapper.find('[data-testid="token-trend-legend"]')
    expect(legend.find('[data-kind="workflow"]').text()).toContain('工作流')
    expect(legend.find('[data-kind="pm"]').text()).toContain('项目管理')
    expect(legend.find('[data-kind="workflow"]').text()).not.toMatch(/^\s*workflow\s*$/i)
    expect(legend.find('[data-kind="pm"]').text()).not.toMatch(/^\s*pm\s*$/i)
    // ECharts renders canvas inside the chart host
    expect(wrapper.find('[data-testid="token-trend-chart"] canvas').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-trend-svg"]').exists()).toBe(false)
    expect(wrapper.html()).not.toMatch(/preserveAspectRatio\s*=\s*["']none["']/)
    const exposed = wrapper.vm as unknown as {
      chartData: { datasets: { label: string; data: number[] }[] }
    }
    expect(exposed.chartData.datasets).toHaveLength(2)
    expect(exposed.chartData.datasets.map((d) => d.label)).toEqual(['workflow', 'pm'])
    wrapper.unmount()
  })

  it('tooltip source names use i18n labels with data-tip-row (g2.2/g3.1)', async () => {
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
        ],
      },
      global: { plugins: [i18n()] },
      attachTo: document.body,
    })
    const wrapEl = wrapper.find('[data-testid="token-trend-wrap"]').element as HTMLElement
    const exposed = wrapper.vm as unknown as {
      chartOptions: {
        plugins?: { tooltip?: { external?: (ctx: unknown) => void } }
      }
    }
    const external = exposed.chartOptions.plugins?.tooltip?.external
    expect(typeof external).toBe('function')
    const canvas = document.createElement('canvas')
    wrapEl.appendChild(canvas)
    Object.defineProperty(canvas, 'getBoundingClientRect', {
      value: () => ({ left: 0, top: 0, width: 200, height: 200, right: 200, bottom: 200 }),
    })
    Object.defineProperty(wrapEl, 'getBoundingClientRect', {
      value: () => ({ left: 0, top: 0, width: 400, height: 200, right: 400, bottom: 200 }),
    })
    external?.({
      chart: { canvas },
      tooltip: { opacity: 1, dataPoints: [{ dataIndex: 0 }], caretX: 40, caretY: 40 },
    })
    await wrapper.vm.$nextTick()
    const tip = document.querySelector('[data-testid="token-trend-tooltip"]')
    expect(tip).toBeTruthy()
    const workflowRow = tip?.querySelector('[data-tip-row="workflow"]')
    const pmRow = tip?.querySelector('[data-tip-row="pm"]')
    expect(workflowRow?.textContent).toContain('工作流')
    expect(pmRow?.textContent).toContain('项目管理')
    expect(workflowRow?.textContent).not.toMatch(/\bworkflow\b/)
    expect(pmRow?.textContent).not.toMatch(/(?<![A-Za-z0-9_-])pm(?![A-Za-z0-9_-])/i)
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
    // Wide container must still host ECharts canvas
    ;(wrap.element as HTMLElement).style.width = '1280px'
    expect(wrapper.find('[data-testid="token-trend-chart"] canvas').exists()).toBe(true)
    wrapper.unmount()
  })

  it('trend wrap uses min-w-0 + overflow-x-clip to prevent ECharts flex overflow (g3.1)', () => {
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
    expect(wrapper.find('[data-testid="token-donut-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('输入')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('输出')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('缓存读')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('缓存写')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).not.toMatch(/\binput\b/)
    // g3.3: share math unchanged for 70/55/35/20 over total 180
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('38.9%')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('30.6%')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('19.4%')
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('11.1%')
    expect(wrapper.text()).toContain('总量')
    expect(wrapper.find('[data-testid="token-donut-chart"]').text()).toContain('180')
    expect(wrapper.find('[data-testid="token-donut-chart"]').attributes('aria-label')).toBe('用量构成')
    wrapper.unmount()
  })

  it('donut tooltip uses the same part* labels as the legend (g2.2)', async () => {
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
    const legend = wrapper.find('[data-testid="token-donut-legend"]')
    expect(legend.find('li').exists()).toBe(true)
    await legend.find('li').trigger('click', { clientX: 40, clientY: 40 })
    const tip = wrapper.find('[data-testid="token-donut-tooltip"]')
    expect(tip.exists()).toBe(true)
    expect(tip.text()).toContain('输入')
    expect(tip.text()).not.toMatch(/\binput\b/)
    expect(tip.text()).toContain('38.9%')
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
    const chart = wrapper.find('[data-testid="token-donut-chart"]')
    expect(chart.classes()).toEqual(expect.arrayContaining(['h-[120px]', 'w-[120px]', 'sm:h-[150px]', 'sm:w-[150px]']))
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
    expect(wrapper.findAll('[data-testid="token-rank-bar"]').length).toBe(4)
    expect(wrapper.findAll('[data-testid="mock-vchart"]').length).toBe(4)
    expect(wrapper.text()).toContain('项目管理')
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

    // Same purple bar as workflow — gradient #6d5cff → #9b8cff
    const pmBar = pmRow.find('[data-testid="token-rank-bar"]')
    expect(pmBar.exists()).toBe(true)
    const barColorFn = (wrapper.vm as { barColor: (w: { kind?: string; other?: boolean }) => { colorStops: { color: string }[] } }).barColor
    const pmGradient = barColorFn({ kind: 'pm' })
    expect(pmGradient.colorStops[0]!.color).toBe('#6d5cff')
    expect(pmGradient.colorStops[1]!.color).toBe('#9b8cff')
    expect(pmGradient.colorStops.map((s) => s.color)).not.toContain('#f59e0b')

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

type ExternalTooltip = (ctx: {
  chart: { canvas: HTMLCanvasElement }
  tooltip: {
    opacity: number
    caretX: number
    caretY: number
    dataPoints?: { dataIndex: number }[]
  }
}) => void | Promise<void>

const TIP_W = 168
const TIP_H = 72

function mockTipBoxSize() {
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
    configurable: true,
    get() {
      return (this as HTMLElement).getAttribute?.('data-testid') === 'token-trend-tooltip' ? TIP_W : 600
    },
  })
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
    configurable: true,
    get() {
      return (this as HTMLElement).getAttribute?.('data-testid') === 'token-trend-tooltip' ? TIP_H : 200
    },
  })
}

function restoreTipBoxSize() {
  delete (HTMLElement.prototype as { offsetWidth?: unknown }).offsetWidth
  delete (HTMLElement.prototype as { offsetHeight?: unknown }).offsetHeight
}

function leftZeroTrend() {
  return [
    {
      bucket: '2026-07-11',
      total: 0,
      workflowTotal: 0,
      pmTotal: 0,
      inputTokens: 0,
      outputTokens: 0,
      cacheReadTokens: 0,
      cacheWriteTokens: 0,
    },
    {
      bucket: '2026-07-12',
      total: 1_200_000,
      workflowTotal: 900_000,
      pmTotal: 300_000,
      inputTokens: 400,
      outputTokens: 300,
      cacheReadTokens: 200,
      cacheWriteTokens: 100,
    },
    {
      bucket: '2026-08-09',
      total: 96_000_000,
      workflowTotal: 72_000_000,
      pmTotal: 24_000_000,
      inputTokens: 400,
      outputTokens: 300,
      cacheReadTokens: 200,
      cacheWriteTokens: 100,
    },
  ]
}

function tipBoxInViewport(el: HTMLElement) {
  const left = parseFloat(el.style.left || '0')
  const top = parseFloat(el.style.top || '0')
  const right = left + (el.offsetWidth || TIP_W)
  const bottom = top + (el.offsetHeight || TIP_H)
  const vw = window.innerWidth
  const vh = window.innerHeight
  expect(left).toBeGreaterThanOrEqual(0)
  expect(top).toBeGreaterThanOrEqual(0)
  expect(right).toBeLessThanOrEqual(vw)
  expect(bottom).toBeLessThanOrEqual(vh)
  return { left, top, right, bottom }
}

async function fireExternalTooltip(
  wrapper: ReturnType<typeof mount>,
  opts: { caretX: number; caretY: number; dataIndex?: number; opacity?: number; canvasLeft?: number; canvasTop?: number },
) {
  const canvas = wrapper.find('canvas').element as HTMLCanvasElement
  const canvasLeft = opts.canvasLeft ?? 40
  const canvasTop = opts.canvasTop ?? 80
  vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
    x: canvasLeft,
    y: canvasTop,
    left: canvasLeft,
    top: canvasTop,
    right: canvasLeft + 600,
    bottom: canvasTop + 178,
    width: 600,
    height: 178,
    toJSON() {
      return {}
    },
  } as DOMRect)

  const vm = wrapper.vm as unknown as { externalTooltip: ExternalTooltip; hideTip: () => void }
  await vm.externalTooltip({
    chart: { canvas },
    tooltip: {
      opacity: opts.opacity ?? 1,
      caretX: opts.caretX,
      caretY: opts.caretY,
      dataPoints: opts.opacity === 0 ? [] : [{ dataIndex: opts.dataIndex ?? 0 }],
    },
  })
  await nextTick()
  await nextTick()
  return vm
}

describe('TokenTrendChart tooltip visibility (g4.1/g4.2/g4.3/g1.1)', () => {
  afterEach(() => {
    restoreTipBoxSize()
    vi.restoreAllMocks()
    document.querySelectorAll('[data-testid="token-trend-tooltip"]').forEach((n) => n.remove())
  })

  it('left-edge total=0 shows full MM-DD · 0 and box stays in viewport (g4.1)', async () => {
    mockTipBoxSize()
    const wrapper = mount(TokenTrendChart, {
      props: { bucketWidth: 'day', trend: leftZeroTrend() },
      global: { plugins: [i18n()] },
      attachTo: document.body,
    })

    await fireExternalTooltip(wrapper, { caretX: 8, caretY: 160, dataIndex: 0 })
    const tip = document.querySelector('[data-testid="token-trend-tooltip"]') as HTMLElement | null
    expect(tip).toBeTruthy()
    const text = (tip!.textContent || '').replace(/\s+/g, ' ')
    expect(text).toContain('07-11')
    expect(text).toMatch(/07-11\s*·\s*0/)
    expect(text).not.toMatch(/(^|\s)-11\s*·/)
    expect(text).toContain('工作流')
    expect(text).toContain('项目管理')
    tipBoxInViewport(tip!)

    const wrap = wrapper.find('[data-testid="token-trend-wrap"]').element
    expect(wrap.contains(tip!)).toBe(false)
    expect(tip!.className).toMatch(/fixed/)
    expect(tip!.className).toMatch(/z-\[100\]/)
    expect(tip!.className).toMatch(/pointer-events-none/)
    expect(tip!.className).toMatch(/rounded-lg/)
    expect(tip!.className).toMatch(/bg-\[#1a1d23\]/)
    wrapper.unmount()
  })

  it('right-edge / high caret flip or clamp keeps box in viewport (g4.2)', async () => {
    mockTipBoxSize()
    const wrapper = mount(TokenTrendChart, {
      props: { bucketWidth: 'day', trend: leftZeroTrend() },
      global: { plugins: [i18n()] },
      attachTo: document.body,
    })

    await fireExternalTooltip(wrapper, { caretX: 590, caretY: 8, dataIndex: 2, canvasLeft: 40, canvasTop: 20 })
    const tip = document.querySelector('[data-testid="token-trend-tooltip"]') as HTMLElement | null
    expect(tip).toBeTruthy()
    const text = (tip!.textContent || '').replace(/\s+/g, ' ')
    expect(text).toContain('08-09')
    expect(text).not.toMatch(/translate\(-50%/)
    tipBoxInViewport(tip!)
    wrapper.unmount()
  })

  it('hides tooltip after leaving the trend wrap (g4.3)', async () => {
    mockTipBoxSize()
    const wrapper = mount(TokenTrendChart, {
      props: { bucketWidth: 'day', trend: leftZeroTrend() },
      global: { plugins: [i18n()] },
      attachTo: document.body,
    })

    await fireExternalTooltip(wrapper, { caretX: 80, caretY: 100, dataIndex: 1 })
    expect(document.querySelector('[data-testid="token-trend-tooltip"]')).toBeTruthy()

    await wrapper.find('[data-testid="token-trend-wrap"]').trigger('mouseleave')
    await nextTick()
    expect(document.querySelector('[data-testid="token-trend-tooltip"]')).toBeNull()

    await fireExternalTooltip(wrapper, { caretX: 80, caretY: 100, dataIndex: 1 })
    expect(document.querySelector('[data-testid="token-trend-tooltip"]')).toBeTruthy()
    await fireExternalTooltip(wrapper, { caretX: 80, caretY: 100, opacity: 0 })
    expect(document.querySelector('[data-testid="token-trend-tooltip"]')).toBeNull()
    wrapper.unmount()
  })
})

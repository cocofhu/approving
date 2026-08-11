// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run, WFNode } from '@/lib/shared/types'
import ExecutionStatsPanel from './ExecutionStatsPanel.vue'

const apiMocks = vi.hoisted(() => ({
  listRuns: vi.fn(),
  getRun: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listRuns: apiMocks.listRuns,
      getRun: apiMocks.getRun,
    },
  }
})

const nodes: WFNode[] = [
  { id: 'research', type: 'research', label: '调研', position: { x: 0, y: 0 }, config: {} },
  { id: 'react', type: 'react', label: '澄清', position: { x: 0, y: 0 }, config: {} },
]

function baseRun(withUsage = false): Run {
  return {
    id: 'run-1',
    title: 'run',
    workflowId: 'wf-1',
    workflowName: 'wf',
    status: 'completed',
    createdAt: '2026-07-18T00:00:00Z',
    startedAt: '2026-07-18T00:00:00Z',
    durationSec: 120,
    nodes: [],
    edges: [],
    nodeStates: {},
    artifacts: [],
    nodeExecutions: {
      research: [
        {
          nodeId: 'research',
          iteration: 1,
          status: 'completed',
          durationSec: 40,
          startedAt: '2026-07-18T00:00:00Z',
          outputs: {},
          ...(withUsage
            ? {
                usage: {
                  inputTokens: 1000,
                  outputTokens: 200,
                  cacheReadTokens: 0,
                  cacheWriteTokens: 0,
                },
              }
            : {}),
        },
      ],
      react: [
        {
          nodeId: 'react',
          iteration: 1,
          status: 'completed',
          durationSec: 30,
          startedAt: '2026-07-18T00:01:00Z',
          outputs: {},
          ...(withUsage
            ? {
                usage: {
                  inputTokens: 100,
                  outputTokens: 50,
                  cacheReadTokens: 10,
                  cacheWriteTokens: 0,
                },
              }
            : {}),
        },
      ],
    },
  } as unknown as Run
}

function mountPanel(run = baseRun()) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ExecutionStatsPanel, {
    props: {
      run,
      nodes,
      wallSec: 120,
      nowMs: Date.parse('2026-07-18T00:02:00Z'),
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, TruncatedTextTooltip: true, StatsPieChart: false },
    },
  })
}

describe('ExecutionStatsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listRuns.mockResolvedValue({
      items: [
        { id: 'run-1', durationSec: 120, startedAt: '2026-07-18T00:00:00Z', createdAt: '2026-07-18T00:00:00Z' },
        { id: 'run-2', durationSec: 90, startedAt: '2026-07-17T00:00:00Z', createdAt: '2026-07-17T00:00:00Z' },
      ],
      total: 2,
    })
    apiMocks.getRun.mockResolvedValue(baseRun())
  })

  it('mounts single-run stats tab by default', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="execution-stats-panel"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('单次 Run 统计')
    expect(wrapper.find('[data-testid="stats-pie-query"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('switches to multi-run tab and loads candidates', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const multiBtn = wrapper.findAll('button').find((b) => b.text().includes('多次执行聚合'))
    expect(multiBtn).toBeTruthy()
    await multiBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.listRuns).toHaveBeenCalled()
    // g1.1 / g1.2 / g1.3: compare bar shows selected; picker collapsed; title N / total
    expect(wrapper.find('[data-testid="stats-multi-compare-bar"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="stats-multi-compare-cell"]').length).toBe(2)
    expect(wrapper.find('[data-testid="stats-selected-runs-title"]').text()).toContain('已选 Run · 2 / 2')
    expect(wrapper.find('[data-testid="stats-multi-picker"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="stats-multi-picker-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.text()).toMatch(/#1|#2/)
    wrapper.unmount()
  })

  it('switches single dimension to node view', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const nodeDim = wrapper.findAll('button').find((b) => b.text().includes('节点'))
    expect(nodeDim).toBeTruthy()
    await nodeDim!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('调研')
    wrapper.unmount()
  })

  it('shows compact token KPIs and ranking when usage is reported', async () => {
    const wrapper = mountPanel(baseRun(true))
    await flushPromises()
    const total = wrapper.find('[data-testid="stats-kpi-total-tokens"]')
    const rate = wrapper.find('[data-testid="stats-kpi-token-rate"]')
    // plan g2.1 / g4.1: compact main values (not full 1,360 / raw rate)
    expect(total.find('[data-testid="stats-kpi-total-tokens-value"]').text()).toContain('1.4K token')
    expect(total.text()).toContain('有用量环节合计')
    expect(rate.find('[data-testid="stats-kpi-token-rate-value"]').text()).toBe('11.3/s')
    expect(rate.text()).toContain('÷ 总耗时')
    expect(wrapper.findAll('[data-testid="stats-rank-tokens"]').length).toBeGreaterThan(0)
    expect(wrapper.text()).not.toContain('NEW')
    wrapper.unmount()
  })

  it('shows dash for token KPIs and ranking when usage is absent', async () => {
    const wrapper = mountPanel(baseRun(false))
    await flushPromises()
    const total = wrapper.find('[data-testid="stats-kpi-total-tokens"]')
    const rate = wrapper.find('[data-testid="stats-kpi-token-rate"]')
    expect(total.find('[data-testid="stats-kpi-total-tokens-value"]').text()).toContain('—')
    expect(total.text()).toContain('全部未上报')
    expect(rate.find('[data-testid="stats-kpi-token-rate-value"]').text()).toContain('—')
    expect(rate.text()).not.toMatch(/\b0\b/)
    const rankTokens = wrapper.findAll('[data-testid="stats-rank-tokens"]')
    expect(rankTokens.every((el) => el.text().includes('—'))).toBe(true)
    wrapper.unmount()
  })

  it('shows compact duration mains and duration tip on hover/focus (g1.1/g3.1/g4.1)', async () => {
    const wrapper = mountPanel(
      {
        ...baseRun(true),
        durationSec: 3703,
        nodeExecutions: {
          research: [
            {
              nodeId: 'research',
              iteration: 1,
              status: 'completed',
              durationSec: 3458,
              startedAt: '2026-07-18T00:00:00Z',
              outputs: {},
              usage: {
                inputTokens: 1245800,
                outputTokens: 892455,
                cacheReadTokens: 7201000,
                cacheWriteTokens: 306000,
              },
            },
          ],
          react: [
            {
              nodeId: 'react',
              iteration: 1,
              status: 'completed',
              durationSec: 0,
              startedAt: '2026-07-18T00:01:00Z',
              outputs: {},
            },
          ],
        },
      } as unknown as Run,
    )
    await wrapper.setProps({ wallSec: 3703 })
    await flushPromises()

    expect(wrapper.find('[data-testid="stats-kpi-wall-value"]').text()).toBe('1.03h')
    expect(wrapper.find('[data-testid="stats-kpi-node-sum-value"]').text()).toBe('57.6m')
    // gap = 3703 - 3458 = 245 → 4.1m
    expect(wrapper.find('[data-testid="stats-kpi-gap-value"]').text()).toBe('4.1m')
    expect(wrapper.find('[data-testid="stats-kpi-total-tokens-value"]').text()).toContain('9.65M token')
    expect(wrapper.find('[data-testid="stats-kpi-token-rate-value"]').text()).toBe('2.60K/s')

    const wallVal = wrapper.find('[data-testid="stats-kpi-wall-value"]')
    await wallVal.trigger('mouseenter')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').text()).toContain('01:01:43')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').text()).toContain('3703 秒')
    await wallVal.trigger('mouseleave')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').exists()).toBe(false)

    await wallVal.trigger('focus')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').exists()).toBe(true)
    await wallVal.trigger('blur')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').exists()).toBe(false)

    const tokenVal = wrapper.find('[data-testid="stats-kpi-total-tokens-value"]')
    await tokenVal.trigger('mouseenter')
    const tip = wrapper.find('[data-testid="stats-kpi-total-tokens-tip"]')
    expect(tip.text()).toContain('9,645,255 token')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-input"]').text()).toMatch(/输入/)
    expect(wrapper.find('[data-testid="stats-kpi-token-part-output"]').text()).toMatch(/输出/)
    expect(wrapper.find('[data-testid="stats-kpi-token-part-cacheRead"]').text()).toMatch(/缓存读/)
    expect(wrapper.find('[data-testid="stats-kpi-token-part-cacheWrite"]').text()).toMatch(/缓存写/)
    expect(wrapper.find('[data-testid="stats-kpi-token-part-cacheRead"]').text()).toContain('7,201,000')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-cacheRead"]').text()).toContain('7.2M')
    await tokenVal.trigger('mouseleave')

    const rateVal = wrapper.find('[data-testid="stats-kpi-token-rate-value"]')
    await rateVal.trigger('mouseenter')
    expect(wrapper.find('[data-testid="stats-kpi-token-rate-tip"]').text()).toMatch(
      /9,645,255 ÷ 3703 秒 ≈ 2604\.7/,
    )
    wrapper.unmount()
  })

  it('keeps four zero rows when usage is absent and degrades rate tip when wall=0 (g3.2/g3.4/g4.1)', async () => {
    const wrapper = mountPanel(baseRun(false))
    await wrapper.setProps({ wallSec: 0 })
    await flushPromises()

    expect(wrapper.find('[data-testid="stats-kpi-wall-value"]').text()).toBe('0s')
    expect(wrapper.find('[data-testid="stats-kpi-token-rate-value"]').text()).toBe('—')

    await wrapper.find('[data-testid="stats-kpi-wall-value"]').trigger('focus')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').text()).toContain('00:00')
    expect(wrapper.find('[data-testid="stats-kpi-wall-tip"]').text()).toContain('0 秒')
    await wrapper.find('[data-testid="stats-kpi-wall-value"]').trigger('blur')

    await wrapper.find('[data-testid="stats-kpi-total-tokens-value"]').trigger('mouseenter')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-input"]').text()).toContain('0')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-output"]').text()).toContain('0')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-cacheRead"]').text()).toContain('0')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-cacheWrite"]').text()).toContain('0')
    await wrapper.find('[data-testid="stats-kpi-total-tokens-value"]').trigger('mouseleave')

    await wrapper.find('[data-testid="stats-kpi-token-rate-value"]').trigger('mouseenter')
    expect(wrapper.find('[data-testid="stats-kpi-token-rate-tip"]').text()).toContain(
      '总时长为 0 秒，无法按耗时计算',
    )
    expect(wrapper.find('[data-testid="stats-kpi-token-rate-tip"]').text()).not.toContain('÷')
    wrapper.unmount()
  })

  it('multi-run token KPIs ignore runs without usage in average denominator', async () => {
    const withUsage = baseRun(true)
    const withoutUsage = {
      ...baseRun(false),
      id: 'run-2',
      durationSec: 90,
      startedAt: '2026-07-17T00:00:00Z',
    } as Run
    apiMocks.getRun.mockImplementation(async (id: string) => {
      if (id === 'run-2') return withoutUsage
      return withUsage
    })
    const wrapper = mountPanel(withUsage)
    await flushPromises()
    const multiBtn = wrapper.findAll('button').find((b) => b.text().includes('多次执行聚合'))
    await multiBtn!.trigger('click')
    await flushPromises()
    // g2.2 / g3.1 / g5.1: compact mains + spoken hints; denom still usageRunCount=1
    const sum = wrapper.find('[data-testid="stats-kpi-sum-tokens"]')
    const avg = wrapper.find('[data-testid="stats-kpi-avg-tokens"]')
    expect(wrapper.find('[data-testid="stats-kpi-sum-tokens-value"]').text()).toContain('1.4K token')
    expect(sum.text()).toContain('只统计有上报用量的执行')
    expect(wrapper.find('[data-testid="stats-kpi-avg-tokens-value"]').text()).toContain('1.4K token')
    expect(avg.text()).toContain('按 1 次有用量的执行取平均')
    expect(wrapper.find('[data-testid="stats-kpi-multi-token-rate"]').text()).toContain(
      '总 token ÷ 所选执行总耗时',
    )
    expect(wrapper.find('[data-testid="stats-kpi-process-count"]').text()).toContain('过程执行次数')
    expect(wrapper.text()).not.toMatch(/\busage\b/i)
    expect(wrapper.text()).not.toContain('分母=')
    expect(wrapper.text()).not.toContain('Σ')
    wrapper.unmount()
  })

  it('multi-run picker opens by day group; current run cannot be deselected (g1.3/g1.4/g1.5)', async () => {
    const now = new Date()
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 10, 30).toISOString()
    const ydayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 18, 5).toISOString()
    const olderStart = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 3, 11, 5).toISOString()
    apiMocks.listRuns.mockResolvedValue({
      items: [
        { id: 'run-1', durationSec: 120, startedAt: todayStart, createdAt: todayStart },
        { id: 'run-2', durationSec: 90, startedAt: ydayStart, createdAt: ydayStart },
        { id: 'run-3', durationSec: 60, startedAt: olderStart, createdAt: olderStart },
        { id: 'run-4', durationSec: 30, startedAt: '', createdAt: '' },
      ],
      total: 4,
    })
    apiMocks.getRun.mockImplementation(async (id: string) => ({
      ...baseRun(id === 'run-1'),
      id,
    }))

    const wrapper = mountPanel({ ...baseRun(true), startedAt: todayStart })
    await flushPromises()
    const multiBtn = wrapper.findAll('button').find((b) => b.text().includes('多次执行聚合'))
    await multiBtn!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="stats-selected-runs-title"]').text()).toContain('已选 Run · 3 / 4')
    expect(wrapper.find('[data-testid="stats-multi-picker"]').exists()).toBe(false)

    await wrapper.find('[data-testid="stats-multi-picker-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="stats-multi-picker"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="stats-multi-picker-toggle"]').attributes('aria-expanded')).toBe('true')

    const groupText = wrapper
      .findAll('[data-testid="stats-multi-day-group"]')
      .map((g) => g.text())
      .join(' | ')
    expect(groupText).toContain('今天')
    expect(groupText).toContain('昨天')
    expect(groupText).toContain('时间未知')
    expect(groupText).toMatch(/\d+\s*月\s*\d+\s*日/)

    const pickerRows = wrapper.findAll('[data-testid="stats-multi-picker-row"]')
    expect(pickerRows.length).toBe(4)

    const currentRow = wrapper.find('[data-testid="stats-multi-picker-row"][data-run-id="run-1"]')
    expect(currentRow.attributes('aria-pressed')).toBe('true')
    await currentRow.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="stats-kpi-selected-value"]').text()).toBe('3')
    expect(wrapper.find('[data-testid="stats-multi-picker-row"][data-run-id="run-1"]').attributes('aria-pressed')).toBe(
      'true',
    )
    expect(wrapper.find('[data-testid="stats-multi-compare-cell"][data-run-id="run-1"]').attributes('data-current')).toBe(
      'true',
    )
    expect(wrapper.findAll('[data-testid="stats-multi-remove"]').length).toBe(2)

    await wrapper.find('[data-testid="stats-multi-picker-row"][data-run-id="run-2"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="stats-kpi-selected-value"]').text()).toBe('2')
    expect(wrapper.find('[data-testid="stats-selected-runs-title"]').text()).toContain('已选 Run · 2 / 4')
    expect(wrapper.findAll('[data-testid="stats-multi-compare-cell"]').length).toBe(2)
    // g1.3: toggling does not auto-collapse picker
    expect(wrapper.find('[data-testid="stats-multi-picker"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('multi-run compact avg wall / token tips and rank 合计 (g2.3/g3.1/g3.2)', async () => {
    const withUsage = baseRun(true)
    const withoutUsage = {
      ...baseRun(false),
      id: 'run-2',
      durationSec: 90,
      startedAt: '2026-07-17T00:00:00Z',
    } as Run
    apiMocks.getRun.mockImplementation(async (id: string) => {
      if (id === 'run-2') return withoutUsage
      return withUsage
    })
    const wrapper = mountPanel(withUsage)
    await flushPromises()
    const multiBtn = wrapper.findAll('button').find((b) => b.text().includes('多次执行聚合'))
    await multiBtn!.trigger('click')
    await flushPromises()

    // avg wall of 120 + 90 = 105s → 1.8m; tip exact clock
    expect(wrapper.find('[data-testid="stats-kpi-avg-wall-value"]').text()).toBe('1.8m')
    await wrapper.find('[data-testid="stats-kpi-avg-wall-value"]').trigger('mouseenter')
    expect(wrapper.find('[data-testid="stats-kpi-avg-wall-tip"]').text()).toContain('01:45')
    await wrapper.find('[data-testid="stats-kpi-avg-wall-value"]').trigger('mouseleave')

    await wrapper.find('[data-testid="stats-kpi-sum-tokens-value"]').trigger('focus')
    expect(wrapper.find('[data-testid="stats-kpi-sum-tokens-tip"]').text()).toContain('1,360 token')
    expect(wrapper.find('[data-testid="stats-kpi-token-part-input"]').exists()).toBe(false)
    await wrapper.find('[data-testid="stats-kpi-sum-tokens-value"]').trigger('blur')

    await wrapper.find('[data-testid="stats-kpi-multi-token-rate-value"]').trigger('mouseenter')
    expect(wrapper.find('[data-testid="stats-kpi-multi-token-rate-tip"]').text()).toMatch(/1,360 ÷ 210 秒/)
    expect(wrapper.find('[data-testid="stats-kpi-multi-token-rate-value"]').text()).toMatch(/\/s/)

    expect(wrapper.text()).toContain('合计')
    expect(wrapper.text()).toContain('对所选 Run 中同一维度的条目合计耗时')
    expect(wrapper.text()).not.toContain('Σ')
    wrapper.unmount()
  })
})

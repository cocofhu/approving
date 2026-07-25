// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run, WFNode } from '@/lib/types'
import ExecutionStatsPanel from './ExecutionStatsPanel.vue'

const apiMocks = vi.hoisted(() => ({
  listRuns: vi.fn(),
  getRun: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
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
    expect(wrapper.text()).toMatch(/run-2|#2/)
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

  it('shows token KPIs and ranking when usage is reported', async () => {
    const wrapper = mountPanel(baseRun(true))
    await flushPromises()
    const total = wrapper.find('[data-testid="stats-kpi-total-tokens"]')
    const rate = wrapper.find('[data-testid="stats-kpi-token-rate"]')
    expect(total.text()).toContain('1,360')
    expect(total.text()).toContain('有用量环节合计')
    expect(rate.text()).toMatch(/11\.3|11\.333/)
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
    expect(total.text()).toContain('—')
    expect(total.text()).toContain('全部未上报')
    expect(rate.text()).toContain('—')
    expect(rate.text()).not.toMatch(/\b0\b/)
    const rankTokens = wrapper.findAll('[data-testid="stats-rank-tokens"]')
    expect(rankTokens.every((el) => el.text().includes('—'))).toBe(true)
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
    // default selection includes run-1 + run-2
    const sum = wrapper.find('[data-testid="stats-kpi-sum-tokens"]')
    const avg = wrapper.find('[data-testid="stats-kpi-avg-tokens"]')
    expect(sum.text()).toContain('1,360')
    expect(sum.text()).toContain('仅合计有 usage')
    expect(avg.text()).toContain('1,360')
    expect(avg.text()).toContain('分母=有用量 Run 数 1')
    wrapper.unmount()
  })
})

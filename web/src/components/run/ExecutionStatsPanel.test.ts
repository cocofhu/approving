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

function baseRun(): Run {
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
})

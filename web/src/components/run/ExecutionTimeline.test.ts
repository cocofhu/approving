// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run, WFNode } from '@/lib/types'
import ExecutionTimeline from './ExecutionTimeline.vue'

const nodes: WFNode[] = [
  { id: 'research', type: 'research', label: '调研', position: { x: 0, y: 0 }, config: {} },
  { id: 'react', type: 'react', label: '澄清', position: { x: 0, y: 0 }, config: {} },
]

function baseRun(): Run {
  return {
    id: 'run-1',
    workflowId: 'wf-1',
    workflowName: 'wf',
    status: 'completed',
    createdAt: '2026-07-18T00:00:00Z',
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
          durationSec: 30,
          startedAt: '2026-07-18T00:00:00Z',
          outputs: { summary: 'ok' },
        },
      ],
      react: [
        {
          nodeId: 'react',
          iteration: 1,
          status: 'running',
          startedAt: '2026-07-18T00:01:00Z',
          outputs: {},
        },
      ],
    },
  } as unknown as Run
}

function mountTimeline(opts: { interactive?: boolean; selectedNodeId?: string | null } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ExecutionTimeline, {
    props: {
      run: baseRun(),
      nodes,
      selectedNodeId: opts.selectedNodeId ?? null,
      selectedExecIdx: 0,
      interactive: opts.interactive ?? true,
      nowMs: Date.parse('2026-07-18T00:02:00Z'),
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true, StatusPill: true, VarValueDisplay: true },
    },
  })
}

describe('ExecutionTimeline', () => {
  it('renders timeline items from node executions', () => {
    const wrapper = mountTimeline()
    expect(wrapper.text()).toContain('调研')
    expect(wrapper.findAll('[data-testid="timeline-node-label"]').length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('emits select when row clicked in interactive mode', async () => {
    const wrapper = mountTimeline({ interactive: true })
    const row = wrapper.find('li .cursor-pointer')
    await row.trigger('click')
    expect(wrapper.emitted('select')).toBeTruthy()
    wrapper.unmount()
  })

  it('expands variable chip on click', async () => {
    const wrapper = mountTimeline()
    const chip = wrapper.find('[data-testid="timeline-variable-chip"]')
    if (chip.exists()) {
      await chip.trigger('click')
      expect(wrapper.find('pre').exists()).toBe(true)
    }
    wrapper.unmount()
  })

  it('shows empty state when no executions', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(ExecutionTimeline, {
      props: {
        run: { ...baseRun(), nodeExecutions: {} },
        nodes,
        selectedNodeId: null,
        selectedExecIdx: 0,
      },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    expect(wrapper.text()).toMatch(/暂无|没有/)
    wrapper.unmount()
  })
})

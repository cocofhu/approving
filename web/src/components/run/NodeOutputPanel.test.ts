// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { NodeRun, Run, WFNode } from '@/lib/shared/types'
import NodeOutputPanel from './NodeOutputPanel.vue'

const apiMocks = vi.hoisted(() => ({
  artifactContent: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      artifactContent: apiMocks.artifactContent,
    },
  }
})

function mountPanel(node: WFNode, nodeRun: NodeRun, run: Run) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(NodeOutputPanel, {
    props: { node, nodeRun, run },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        StatusPill: true,
        CompositeVarBlock: true,
        PlanView: true,
        AppPreviewPanel: true,
        OutputResultCards: true,
      },
    },
  })
}

describe('NodeOutputPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.artifactContent.mockResolvedValue({ content: '{"goals":[]}' })
  })

  it('renders input node parameters', async () => {
    const node: WFNode = {
      id: 'input-1',
      type: 'input',
      label: '输入',
      position: { x: 0, y: 0 },
      config: { variables: [{ name: 'topic', desc: '主题' }] },
    }
    const nodeRun: NodeRun = {
      nodeId: 'input-1',
      iteration: 1,
      status: 'completed',
      outputs: { topic: 'hello' },
    }
    const run = { id: 'run-1', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.text()).toContain('主题')
    expect(wrapper.text()).toContain('topic')
    wrapper.unmount()
  })

  it('renders research agent info and structured markdown', async () => {
    const node: WFNode = {
      id: 'research',
      type: 'research',
      label: '调研',
      position: { x: 0, y: 0 },
      config: { skill_profile: 'researcher' },
    }
    const nodeRun: NodeRun = {
      nodeId: 'research',
      iteration: 1,
      status: 'completed',
      durationSec: 42,
      outputs: {
        narration_summary: '完成调研',
        research: '## 调研结论\n内容',
      },
    }
    const run = { id: 'run-1', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.text()).toContain('researcher')
    expect(wrapper.text()).toContain('完成调研')
    wrapper.unmount()
  })

  it('loads plan.json for plan node', async () => {
    const planDoc = { title: '计划', goals: [{ id: 'g1', title: '目标', status: 'pending' }] }
    const node: WFNode = {
      id: 'plan',
      type: 'plan',
      label: '计划',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'plan',
      iteration: 1,
      status: 'completed',
      outputs: { plan_json: JSON.stringify(planDoc) },
    }
    const run = {
      id: 'run-1',
      artifacts: [{ id: 'a1', name: 'plan.json', kind: 'json', nodeId: 'plan', runId: 'run-1', workflowName: 'wf', sizeBytes: 10, createdAt: '2026-07-18T00:00:00Z' }],
    } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PlanView' }).exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows Demo empty state when completed output node has no cards (g4.4)', async () => {
    const node: WFNode = {
      id: 'end',
      type: 'output',
      label: '结束',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'end',
      iteration: 1,
      status: 'completed',
      outputs: { outputCards: [] },
    }
    const run = { id: 'run-1', status: 'completed', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.get('[data-testid="node-output-empty"]').text()).toContain('本次没有可预览的结果卡')
    expect(wrapper.text()).toContain('这不是加载失败')
    expect(wrapper.text()).toContain('Artifacts Tab')
    expect(wrapper.text()).not.toContain('该节点尚未执行')
    wrapper.unmount()
  })

  it('scroll root has no p-4 so output detail bar can sticky top-0 (g3.1)', async () => {
    const node: WFNode = {
      id: 'end',
      type: 'output',
      label: '结束',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = {
      nodeId: 'end',
      iteration: 1,
      status: 'completed',
      outputs: {
        outputCards: [
          {
            index: 1,
            template: 'research',
            title: '调研',
            status: 'ok',
            typeTag: 'Markdown',
            markdown: 'body',
          },
        ],
      },
    }
    const run = { id: 'run-1', status: 'completed', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    const scroll = wrapper.get('[data-testid="node-output-scroll"]')
    expect(scroll.classes()).not.toContain('p-4')
    expect(scroll.find('.p-4').exists()).toBe(true)
    expect(wrapper.find('[data-testid="node-output-empty"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows Demo empty state when run completed but output node never ran (g4.4)', async () => {
    const node: WFNode = {
      id: 'end',
      type: 'output',
      label: '结束',
      position: { x: 0, y: 0 },
      config: {},
    }
    const nodeRun: NodeRun = { nodeId: 'end', status: 'pending', outputs: {} }
    const run = { id: 'run-1', status: 'completed', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.get('[data-testid="node-output-empty"]').text()).toContain('本次没有可预览的结果卡')
    expect(wrapper.text()).not.toContain('该节点尚未执行')
    wrapper.unmount()
  })

  it('renders AppPreviewPanel for app_preview (not the generic agent card)', async () => {
    const node: WFNode = {
      id: 'preview',
      type: 'app_preview',
      label: '预览',
      position: { x: 0, y: 0 },
      config: { skill_profile: 'previewer' },
    }
    const nodeRun: NodeRun = {
      nodeId: 'preview',
      iteration: 1,
      status: 'waiting_human',
      outputs: {},
    }
    const run = { id: 'run-1', artifacts: [] } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'AppPreviewPanel' }).exists()).toBe(true)
    expect(wrapper.text()).toContain('应用预览')
    wrapper.unmount()
  })

  it('renders clarify interaction card for approve nodes', async () => {
    const node: WFNode = {
      id: 'approve',
      type: 'approve',
      label: 'Approve',
      position: { x: 0, y: 0 },
      config: { skill_profile: 'pm' },
    }
    const nodeRun: NodeRun = {
      nodeId: 'approve',
      iteration: 1,
      status: 'completed',
      outputs: { clarified_requirement: '结论正文' },
    }
    const run = {
      id: 'run-1',
      artifacts: [],
      clarifyByNode: {
        approve: { nodeId: 'approve', turns: [], done: true },
      },
    } as unknown as Run
    const wrapper = mountPanel(node, nodeRun, run)
    await flushPromises()
    expect(wrapper.text()).toContain('pm')
    expect(wrapper.text()).toContain('结论正文')
    wrapper.unmount()
  })
})

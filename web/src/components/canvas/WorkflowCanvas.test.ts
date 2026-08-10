// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { WFEdge, WFNode } from '@/lib/shared/types'

vi.mock('@vue-flow/core', () => ({
  VueFlow: defineComponent({
    name: 'VueFlow',
    props: ['nodes', 'edges', 'nodeTypes', 'edgeTypes', 'fitViewOnInit'],
    template: '<div data-testid="vue-flow"><slot /></div>',
  }),
  useVueFlow: () => ({
    project: (p: { x: number; y: number }) => p,
    vueFlowRef: {
      value: {
        getBoundingClientRect: () => ({ left: 0, top: 0, right: 800, bottom: 600, width: 800, height: 600 }),
      },
    },
  }),
  MarkerType: { ArrowClosed: 'arrowclosed' },
  Handle: defineComponent({ template: '<div />' }),
  Position: { Left: 'left', Right: 'right' },
}))

vi.mock('@vue-flow/background', () => ({
  Background: defineComponent({ template: '<div data-testid="flow-bg" />' }),
}))
vi.mock('@vue-flow/controls', () => ({
  Controls: defineComponent({ template: '<div data-testid="flow-controls" />' }),
}))
vi.mock('@vue-flow/minimap', () => ({
  MiniMap: defineComponent({ template: '<div data-testid="flow-minimap" />' }),
}))

import WorkflowCanvas from './WorkflowCanvas.vue'

function sampleNodes(): WFNode[] {
  return [
    {
      id: 'input',
      type: 'input',
      label: '输入',
      position: { x: 0, y: 0 },
      config: { variables: [] },
    },
    {
      id: 'research',
      type: 'research',
      label: '调研',
      position: { x: 200, y: 0 },
      config: { skill_profile: 'default' },
    },
  ]
}

function sampleEdges(): WFEdge[] {
  return [{ id: 'e1', source: 'input', target: 'research', kind: 'success' }]
}

function mountCanvas(overrides: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(WorkflowCanvas, {
    props: {
      nodes: sampleNodes(),
      edges: sampleEdges(),
      mode: 'edit',
      ...overrides,
    },
    global: {
      plugins: [i18n],
      stubs: {
        BaseNode: true,
        ConditionEdge: true,
      },
    },
  })
}

describe('WorkflowCanvas', () => {
  it('mounts vue-flow with background and controls in edit mode', () => {
    const wrapper = mountCanvas()
    expect(wrapper.find('[data-testid="vue-flow"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="flow-bg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="flow-controls"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('computes flow nodes from wf nodes in run mode', () => {
    const wrapper = mountCanvas({
      mode: 'run',
      statusMap: { research: 'running' },
      selectedNode: 'research',
    })
    expect(wrapper.find('[data-testid="vue-flow"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('derives gate action edges from human_gate config', () => {
    const nodes: WFNode[] = [
      {
        id: 'gate',
        type: 'human_gate',
        label: '门禁',
        position: { x: 0, y: 0 },
        config: {
          actions: [{ id: 'approve', label: '批准', goto: 'out' }],
        },
      },
      {
        id: 'out',
        type: 'output',
        label: '输出',
        position: { x: 200, y: 0 },
        config: {},
      },
    ]
    const wrapper = mountCanvas({ nodes, edges: [] })
    expect(wrapper.find('[data-testid="vue-flow"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('emits connect when vue-flow connects nodes', async () => {
    const wrapper = mountCanvas()
    const vf = wrapper.findComponent({ name: 'VueFlow' })
    await vf.vm.$emit('connect', { source: 'input', target: 'research', sourceHandle: null })
    expect(wrapper.emitted('connect')?.[0]?.[0]).toEqual({
      source: 'input',
      target: 'research',
      sourceHandle: null,
    })
    wrapper.unmount()
  })

  it('emits drop-node on canvas drop with node type', async () => {
    const wrapper = mountCanvas()
    const root = wrapper.find('.h-full')
    const dt = {
      getData: (k: string) => (k === 'application/approving-node' ? 'research' : ''),
      dropEffect: 'move',
    }
    const ev = new Event('drop') as DragEvent
    Object.defineProperty(ev, 'dataTransfer', { value: dt })
    Object.defineProperty(ev, 'clientX', { value: 100 })
    Object.defineProperty(ev, 'clientY', { value: 80 })
    root.element.dispatchEvent(ev)
    const dropped = wrapper.emitted('drop-node')
    expect(dropped).toBeTruthy()
    expect(dropped![0][0]).toMatchObject({ type: 'research', x: 12, y: 56 })
    wrapper.unmount()
  })

  it('emits move-node after node drag stop', async () => {
    const wrapper = mountCanvas()
    const vf = wrapper.findComponent({ name: 'VueFlow' })
    await vf.vm.$emit('nodeDragStop', { node: { id: 'research', position: { x: 10, y: 20 } } })
    expect(wrapper.emitted('move-node')?.[0]?.[0]).toEqual({ id: 'research', x: 10, y: 20 })
    wrapper.unmount()
  })

  it('selects structured gate node when derived sg edge clicked', async () => {
    const nodes: WFNode[] = [
      {
        id: 'test',
        type: 'test',
        label: '测试',
        position: { x: 0, y: 0 },
        config: { exits: { pass: { goto: 'out' } } },
      },
      { id: 'out', type: 'output', label: '输出', position: { x: 200, y: 0 }, config: {} },
    ]
    const wrapper = mountCanvas({ nodes, edges: [] })
    const vf = wrapper.findComponent({ name: 'VueFlow' })
    await vf.vm.$emit('edgeClick', { edge: { id: 'sg:test:pass', source: 'test' } })
    expect(wrapper.emitted('select-node')?.[0]?.[0]).toBe('test')
    wrapper.unmount()
  })

  it('emits clear-structured-goto when structured edge removed', async () => {
    const wrapper = mountCanvas()
    const vf = wrapper.findComponent({ name: 'VueFlow' })
    await vf.vm.$emit('edgesChange', [{ type: 'remove', id: 'sg:test:pass' }])
    expect(wrapper.emitted('clear-structured-goto')?.[0]?.[0]).toEqual({ edgeId: 'sg:test:pass' })
    wrapper.unmount()
  })
})

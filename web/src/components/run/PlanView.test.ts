// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import PlanView, { type PlanDoc } from './PlanView.vue'

const mermaidRender = vi.fn()

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: (...args: unknown[]) => mermaidRender(...args),
  },
}))

function mountPlan(doc: PlanDoc, accent?: string) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(PlanView, {
    props: { doc, accent },
    global: { plugins: [i18n], stubs: { Icon: true, AnnotateBtn: true } },
  })
}

describe('PlanView', () => {
  beforeEach(() => {
    mermaidRender.mockReset()
    mermaidRender.mockResolvedValue({ svg: '<svg data-ok="1"></svg>' })
  })

  it('renders goals and progress (legacy goals-only)', () => {
    const doc: PlanDoc = {
      title: '实施计划',
      goals: [
        {
          id: 'g1',
          title: '大目标',
          status: 'in_progress',
          subgoals: [
            { id: 'g1.1', title: '子目标 A', status: 'done' },
            { id: 'g1.2', title: '子目标 B', status: 'pending' },
          ],
        },
      ],
    }
    const wrapper = mountPlan(doc)
    expect(wrapper.text()).toContain('实施计划')
    expect(wrapper.text()).toContain('大目标')
    expect(wrapper.text()).toContain('子目标 A')
    expect(wrapper.text()).toContain('1/2')
    expect(wrapper.find('[data-testid="plan-design"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('uses default title when doc.title missing', () => {
    const wrapper = mountPlan({ goals: [{ id: 'g1', title: '仅目标', status: 'pending' }] })
    expect(wrapper.text()).toMatch(/计划|Plan/)
    expect(wrapper.text()).toContain('仅目标')
    wrapper.unmount()
  })

  it('renders full six design sections and keeps progress on goals leaves', async () => {
    const doc: PlanDoc = {
      title: '完整计划',
      architecture: {
        summary: 'arch summary',
        diagram: { format: 'mermaid', source: 'flowchart LR\n  A-->B', caption: '架构' },
      },
      data_design: {
        summary: 'data',
        entities: [{ name: 'planDoc' }],
        diagram: { source: 'erDiagram\n  A ||--o{ B : has' },
      },
      interfaces: [{ name: 'set_plan', summary: '写入' }],
      components: [{ name: 'plan.go', responsibility: 'parse' }],
      interaction: {
        summary: 'flow',
        diagram: { source: 'sequenceDiagram\n  A->>B: hi' },
      },
      test_design: 'S1-S7',
      goals: [
        {
          id: 'g1',
          title: 'G',
          status: 'in_progress',
          subgoals: [
            { id: 'g1.1', title: 'S1', status: 'done' },
            { id: 'g1.2', title: 'S2', status: 'pending' },
            { id: 'g1.3', title: 'S3', status: 'pending' },
          ],
        },
      ],
    }
    const wrapper = mountPlan(doc)
    await flushPromises()
    expect(wrapper.find('[data-testid="plan-design"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-sec-architecture"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-sec-data"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-sec-interfaces"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-sec-components"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-sec-interaction"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="plan-sec-test"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('1/3')
    expect(mermaidRender).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows 不涉及 placeholder for NA sections', () => {
    const doc: PlanDoc = {
      architecture: { summary: '不涉及' },
      data_design: { summary: '不涉及' },
      interfaces: [{ name: '不涉及', summary: '无' }],
      components: [{ name: '不涉及' }],
      interaction: { summary: '不涉及' },
      test_design: '不涉及',
      goals: [{ id: 'g1', title: 'G', status: 'pending' }],
    }
    const wrapper = mountPlan(doc)
    expect(wrapper.find('[data-testid="plan-design"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('不涉及')
    expect(wrapper.find('[data-testid="plan-diagram"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('falls back to source when mermaid render fails', async () => {
    mermaidRender.mockRejectedValueOnce(new Error('boom'))
    const doc: PlanDoc = {
      architecture: {
        summary: 'arch',
        diagram: { source: 'flowchart LR\n  FAIL-->HERE' },
      },
      goals: [{ id: 'g1', title: 'G', status: 'pending' }],
    }
    const wrapper = mountPlan(doc)
    await flushPromises()
    expect(wrapper.find('[data-testid="plan-diagram-fallback"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('FAIL-->HERE')
    expect(wrapper.text()).toContain('G')
    wrapper.unmount()
  })
})

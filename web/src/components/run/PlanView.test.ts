// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import commonZh from '@/locales/zh-CN/common.json'
import pagesZh from '@/locales/zh-CN/pages.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import PlanView, { type PlanDoc } from './PlanView.vue'

const mermaidRender = vi.fn()

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: (...args: unknown[]) => mermaidRender(...args),
  },
}))

const designDoc: PlanDoc = {
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

function mountPlan(doc: PlanDoc, accent?: string, locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n =
    locale === 'zh-CN'
      ? createI18n({
          legacy: false,
          locale: 'zh-CN',
          messages: { 'zh-CN': { ...commonZh, ...pagesZh } },
        })
      : createI18n({
          legacy: false,
          locale: 'en',
          messages: { en: { ...commonEn, ...pagesEn } },
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
    const wrapper = mountPlan(designDoc)
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

  it('zh-CN shows Chinese design section titles (g1.2)', async () => {
    const wrapper = mountPlan(designDoc, undefined, 'zh-CN')
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('架构')
    expect(text).toContain('数据设计')
    expect(text).toContain('接口')
    expect(text).toContain('组件')
    expect(text).toContain('交互')
    expect(text).toContain('测试设计')
    expect(text).not.toContain('Architecture')
    expect(text).not.toContain('Data design')
    expect(text).not.toContain('Interfaces')
    expect(text).not.toContain('Components')
    expect(text).not.toContain('Interaction')
    expect(text).not.toContain('Test design')
    wrapper.unmount()
  })

  it('en shows English design section titles', async () => {
    const wrapper = mountPlan(designDoc, undefined, 'en')
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('Architecture')
    expect(text).toContain('Data design')
    expect(text).toContain('Interfaces')
    expect(text).toContain('Components')
    expect(text).toContain('Interaction')
    expect(text).toContain('Test design')
    expect(text).not.toContain('数据设计')
    expect(text).not.toContain('测试设计')
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

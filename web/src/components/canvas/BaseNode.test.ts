// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import BaseNode from './BaseNode.vue'

vi.mock('@vue-flow/core', () => ({
  Handle: defineComponent({ props: ['type', 'position'], template: '<div data-testid="handle" />' }),
  Position: { Left: 'left', Right: 'right' },
}))

function mountNode(data: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(BaseNode, {
    props: {
      id: 'n1',
      data: {
        type: 'research',
        label: '调研',
        status: 'running',
        ...data,
      },
      selected: false,
    },
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

describe('BaseNode', () => {
  it('renders node label and status badge for running state', () => {
    const wrapper = mountNode()
    expect(wrapper.text()).toContain('调研')
    expect(wrapper.find('[data-testid="handle"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders branch handles when branches are provided', () => {
    const wrapper = mountNode({
      type: 'branch',
      label: '分支',
      branches: [{ id: 'case-0', label: '默认' }],
    })
    expect(wrapper.text()).toContain('分支')
    wrapper.unmount()
  })

  it('app_preview review: badge + subtitle + Demo body, no action handles', () => {
    const wrapper = mountNode({
      type: 'app_preview',
      label: '应用预览',
      appPreviewReview: true,
      status: undefined,
    })
    expect(wrapper.text()).toContain('复审')
    expect(wrapper.text()).toContain('取点标注 · ReAct')
    expect(wrapper.text()).toContain('确认并流转 · 待审批')
    expect(wrapper.text()).not.toContain('无通过/退回')
    expect(wrapper.text()).not.toContain('未设 goto')
    expect(wrapper.find('[data-testid="app-preview-body"]').exists()).toBe(true)
    // No gate action rows
    expect(wrapper.findAll('[data-testid="handle"]').length).toBeGreaterThanOrEqual(2) // target + single source
    wrapper.unmount()
  })

  it('human_gate review: Demo footnote instead of action Handle rows', () => {
    const wrapper = mountNode({
      type: 'human_gate',
      label: '人工门禁',
      humanGateReview: true,
      gateActions: [
        { id: 'approve', label: '批准' },
        { id: 'revise', label: '退回修改' },
      ],
      status: undefined,
    })
    expect(wrapper.text()).toContain('同一套 ReAct')
    expect(wrapper.text()).toContain('确认并流转 · 待审批')
    expect(wrapper.text()).not.toContain('批准')
    expect(wrapper.text()).not.toContain('退回修改')
    expect(wrapper.text()).not.toContain('未设 goto')
    expect(wrapper.find('[data-testid="human-gate-body"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

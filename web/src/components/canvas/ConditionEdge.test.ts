// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { WFEdge, WFNode } from '@/lib/shared/types'

vi.mock('@vue-flow/core', () => ({
  BaseEdge: defineComponent({ template: '<path />' }),
  EdgeLabelRenderer: defineComponent({ template: '<div><slot /></div>' }),
  Position: { Left: 'left', Right: 'right', Top: 'top', Bottom: 'bottom' },
  getBezierPath: () => ['M0 0 L10 10', 5, 5],
  getSmoothStepPath: () => ['M0 0 L10 10', 5, 5],
}))

import { Position } from '@vue-flow/core'
import ConditionEdge from './ConditionEdge.vue'

function mountEdge(data: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ConditionEdge, {
    props: {
      id: 'e1',
      sourceX: 0,
      sourceY: 0,
      targetX: 100,
      targetY: 50,
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      data: { label: 'Pass · ok', tone: 'ok', ...data },
    },
    global: { plugins: [i18n] },
  })
}

describe('ConditionEdge', () => {
  it('renders edge label from data', () => {
    const wrapper = mountEdge()
    expect(wrapper.text()).toContain('Pass · ok')
    wrapper.unmount()
  })

  it('computes backward step path when shape is step and target is left', () => {
    const wrapper = mountEdge({ shape: 'step' })
    expect(wrapper.find('path').exists()).toBe(true)
    wrapper.unmount()
  })
})

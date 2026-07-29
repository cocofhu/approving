// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenModelComposition from './TokenModelComposition.vue'
import { colorForModel } from './tokenModelColors'

const i18n = () =>
  createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })

describe('TokenModelComposition solid pie (g1.1/g1.2/g1.3)', () => {
  it('renders solid pie without inset donut hole (g1.1)', () => {
    const wrapper = mount(TokenModelComposition, {
      props: {
        models: [{ modelKey: '未知/未分桶', name: '未知/未分桶', total: 1056, unknown: true }],
      },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect(pie.exists()).toBe(true)
    const style = (pie.element as HTMLElement).style
    expect(style.background).toMatch(/conic-gradient/i)
    // Demo alignment: solid pie — no inset box-shadow hole
    expect(style.boxShadow).toBe('')
    expect(wrapper.html()).not.toMatch(/inset\s+0\s+0\s+0\s+28px/)
    wrapper.unmount()
  })

  it('unknown-only bucket: 100% solid unknown color #71717A (g1.2)', () => {
    const unknown = { modelKey: '未知/未分桶', name: '未知/未分桶', total: 1056240000, unknown: true }
    expect(colorForModel(unknown, 0)).toBe('#71717A')

    const wrapper = mount(TokenModelComposition, {
      props: { models: [unknown] },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect((pie.element as HTMLElement).style.background).toContain('#71717A')
    const legend = wrapper.find('[data-testid="token-model-legend"]')
    expect(legend.text()).toContain('未知/未分桶')
    expect(legend.text()).toContain('100%')
    expect(legend.text()).toContain('1056.24M')
    // Single-color full circle is expected data presentation, not a style defect
    wrapper.unmount()
  })

  it('multi-bucket: solid sectors keep unknown #71717A and other #A1A1AA (g1.3/g2.1)', () => {
    const models = [
      { modelKey: 'claude-sonnet-4', name: 'claude-sonnet-4', total: 600, filled: true },
      { modelKey: '未知/未分桶', name: '未知/未分桶', total: 300, unknown: true },
      { modelKey: 'other', name: 'other', total: 100, other: true },
    ]
    expect(colorForModel(models[1]!, 1)).toBe('#71717A')
    expect(colorForModel(models[2]!, 2)).toBe('#A1A1AA')

    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    const bg = (pie.element as HTMLElement).style.background
    expect(bg).toMatch(/conic-gradient/i)
    expect(bg).toContain('#71717A')
    expect(bg).toContain('#A1A1AA')
    expect((pie.element as HTMLElement).style.boxShadow).toBe('')
    const legend = wrapper.find('[data-testid="token-model-legend"]').text()
    expect(legend).toContain('未知/未分桶')
    expect(legend).toContain('other')
    expect(legend).toContain('claude-sonnet-4')
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenModelRank from './TokenModelRank.vue'
import { colorForModel } from './tokenModelColors'

const i18n = () =>
  createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })

describe('TokenModelRank unknown vs other (g3.3)', () => {
  it('qualifying unknown keeps 未知/未分桶, data-unknown, gray text and #71717A bar', () => {
    const models = [
      { modelKey: 'claude-sonnet-4', name: 'claude-sonnet-4', total: 100 },
      { modelKey: '未知/未分桶', name: '未知/未分桶', total: 80, unknown: true },
      { name: 'other', total: 30, other: true },
    ]
    expect(colorForModel(models[1]!, 1)).toBe('#71717A')
    expect(colorForModel(models[2]!, 2)).toBe('#A1A1AA')

    const wrapper = mount(TokenModelRank, {
      props: { models },
      global: { plugins: [i18n()] },
    })

    expect(wrapper.html()).not.toMatch(/「未知」与 other 不同/)
    expect(wrapper.html()).not.toMatch(/Unknown is not the same as other/i)
    expect(wrapper.html()).not.toMatch(/相关用量按其实际消耗参与排行/)

    const unk = wrapper.find('[data-unknown="1"]')
    expect(unk.exists()).toBe(true)
    expect(unk.attributes('data-other')).toBe('0')
    expect(unk.text()).toContain('未知/未分桶')
    expect(unk.text()).not.toContain('other（其余模型）')
    expect(unk.find('.text-txt3').exists()).toBe(true)
    const unkBar = unk.find('.h-full')
    expect(unkBar.attributes('style')).toMatch(/#71717A/i)

    const other = wrapper.find('[data-other="1"]')
    expect(other.exists()).toBe(true)
    expect(other.attributes('data-unknown')).toBe('0')
    expect(other.text()).toContain('other（其余模型）')
    const otherBar = other.find('.h-full')
    expect(otherBar.attributes('style')).toMatch(/#A1A1AA/i)

    wrapper.unmount()
  })
})

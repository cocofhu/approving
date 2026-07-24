// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import TruncatedTextTooltip from './TruncatedTextTooltip.vue'

describe('TruncatedTextTooltip', () => {
  it('renders text slot content', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(TruncatedTextTooltip, {
      props: { text: '短文本' },
      slots: { default: '显示文本' },
      global: { plugins: [i18n] },
    })
    expect(wrapper.text()).toContain('显示文本')
    wrapper.unmount()
  })
})

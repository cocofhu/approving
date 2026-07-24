// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import BrandLogo from './BrandLogo.vue'

describe('BrandLogo', () => {
  it('renders app name and tagline', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(BrandLogo, {
      props: { size: 'md', align: 'center' },
      global: { plugins: [i18n] },
    })
    expect(wrapper.find('.brand-logo__name').text().length).toBeGreaterThan(0)
    expect(wrapper.find('.brand-logo__tagline').exists()).toBe(true)
    wrapper.unmount()
  })
})

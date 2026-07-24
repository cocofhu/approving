// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', meta: {} }),
}))

vi.mock('@/lib/useShutdownState', () => ({
  isDraining: () => false,
}))

import AppTopbar from './AppTopbar.vue'

describe('AppTopbar', () => {
  it('renders header with theme toggle', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AppTopbar, {
      global: {
        plugins: [i18n],
        stubs: { Icon: true, LangSelect: { template: '<div data-testid="lang" />' } },
      },
    })
    expect(wrapper.find('header').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lang"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('emits toggle-menu from mobile menu button', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AppTopbar, {
      global: {
        plugins: [i18n],
        stubs: { Icon: true, LangSelect: true },
      },
    })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('toggle-menu')).toHaveLength(1)
    wrapper.unmount()
  })
})

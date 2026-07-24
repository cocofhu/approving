// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import VarValueDisplay from './VarValueDisplay.vue'

function mountDisplay(value: unknown, props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(VarValueDisplay, {
    props: { value, ...props },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('VarValueDisplay', () => {
  it('renders string value', () => {
    const wrapper = mountDisplay('hello')
    expect(wrapper.text()).toContain('hello')
    wrapper.unmount()
  })

  it('localizes boolean when localeBool is true', () => {
    const wrapper = mountDisplay(true, { localeBool: true })
    expect(wrapper.text()).toMatch(/是|否/)
    wrapper.unmount()
  })

  it('shows empty placeholder for null', () => {
    const wrapper = mountDisplay(null)
    expect(wrapper.text().length).toBeGreaterThan(0)
    wrapper.unmount()
  })
})

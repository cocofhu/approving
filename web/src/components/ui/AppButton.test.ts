// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import AppButton from './AppButton.vue'

function mountBtn(props: Record<string, unknown> = {}, slot = '保存') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AppButton, {
    props,
    slots: { default: slot },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('AppButton', () => {
  it('renders slot text', () => {
    const wrapper = mountBtn({}, '提交')
    expect(wrapper.text()).toBe('提交')
    wrapper.unmount()
  })

  it('applies primary variant classes', () => {
    const wrapper = mountBtn({ variant: 'primary' })
    expect(wrapper.classes().join(' ')).toContain('bg-accent')
    wrapper.unmount()
  })
})

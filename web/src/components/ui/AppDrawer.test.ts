// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import AppDrawer from './AppDrawer.vue'

function mountDrawer(open = true) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(AppDrawer, {
    props: { open, title: '抽屉' },
    slots: { default: '<div data-testid="drawer-body">正文</div>' },
    global: { plugins: [i18n], stubs: { Icon: true } },
    attachTo: document.body,
  })
}

describe('AppDrawer', () => {
  it('teleports drawer content when open', () => {
    const wrapper = mountDrawer(true)
    expect(document.body.textContent).toContain('抽屉')
    expect(document.body.querySelector('[data-testid="drawer-body"]')).toBeTruthy()
    wrapper.unmount()
  })

  it('emits close from overlay click', async () => {
    const wrapper = mountDrawer(true)
    const overlay = document.body.querySelector('.bg-black\\/50') as HTMLElement
    overlay?.click()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import { useToast } from '@/lib/composables/useToast'
import ToastHost from './ToastHost.vue'

describe('ToastHost', () => {
  it('renders toast messages from useToast', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common } },
    })
    const wrapper = mount(ToastHost, {
      global: { plugins: [i18n] },
      attachTo: document.body,
    })
    useToast().success('保存成功')
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).toContain('保存成功')
    const host = document.querySelector('[data-testid="toast-host"]')
    expect(host?.getAttribute('role')).toBe('status')
    expect(host?.getAttribute('aria-live')).toBe('polite')
    expect(host?.className).toContain('right-6')
    expect(host?.className).toContain('z-[100]')
    wrapper.unmount()
  })
})

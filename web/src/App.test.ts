// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', meta: { bare: false, titleKey: 'shell.appName' } }),
}))

vi.mock('@/lib/locale', async () => {
  const actual = await vi.importActual<typeof import('@/lib/locale')>('@/lib/locale')
  return {
    ...actual,
    updateDocumentTitle: vi.fn(),
  }
})

import App from '@/App.vue'
import { getAuthState, useAuth } from '@/lib/useAuth'
import { resetRoutePending } from '@/lib/routePending'

describe('App', () => {
  it('mounts shell layout with router-view stub when auth is ready', () => {
    useAuth().setUser({ username: 't', expiresAt: 't' })
    resetRoutePending()
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(App, {
      global: {
        plugins: [i18n],
        stubs: {
          AppShell: defineComponent({
            template: '<div data-testid="shell"><slot /></div>',
          }),
          ToastHost: defineComponent({ template: '<div data-testid="toast-host" />' }),
          RouterView: defineComponent({ template: '<div data-testid="router-view" />' }),
        },
      },
    })
    expect(wrapper.find('[data-testid="shell"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="toast-host"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="router-view"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not mount protected router-view while auth is not ready', () => {
    const authState = getAuthState()
    authState.user = null
    authState.ready = false
    resetRoutePending()
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(App, {
      global: {
        plugins: [i18n],
        stubs: {
          AppShell: defineComponent({
            template: '<div data-testid="shell"><slot /></div>',
          }),
          ToastHost: defineComponent({ template: '<div data-testid="toast-host" />' }),
          RouterView: defineComponent({ template: '<div data-testid="router-view" />' }),
        },
      },
    })
    expect(wrapper.find('[data-testid="shell"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="router-view"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', meta: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/lib/useShutdownState', () => ({
  drainToast: { visible: false, text: '' },
  formatGrace: () => '',
  isDraining: () => false,
  isOffline: () => false,
  shutdownState: { mode: 'normal', graceRemainingSeconds: 0, message: '', checked: true },
  startShutdownPolling: vi.fn(),
  stopShutdownPolling: vi.fn(),
}))

import AppShell from './AppShell.vue'

describe('AppShell', () => {
  it('renders sidebar and topbar with default slot', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AppShell, {
      slots: { default: '<main data-testid="main">内容</main>' },
      global: {
        plugins: [i18n],
        stubs: {
          AppSidebar: defineComponent({ template: '<aside data-testid="sidebar" />' }),
          AppTopbar: defineComponent({ template: '<header data-testid="topbar" />' }),
          AppSidebarNav: true,
          BrandLogo: true,
          Icon: true,
        },
      },
    })
    expect(wrapper.find('[data-testid="sidebar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="topbar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="main"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

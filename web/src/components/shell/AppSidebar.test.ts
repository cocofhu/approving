// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  RouterLink: { template: '<a><slot /></a>' },
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    authApi: { logout: vi.fn().mockResolvedValue(undefined) },
  }
})

import AppSidebar from './AppSidebar.vue'

describe('AppSidebar', () => {
  it('renders brand and nav stubs', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AppSidebar, {
      global: {
        plugins: [i18n],
        stubs: {
          BrandLogo: { template: '<div data-testid="brand" />' },
          AppSidebarNav: { template: '<nav data-testid="nav" />' },
        },
      },
    })
    expect(wrapper.find('[data-testid="brand"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="nav"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import { __resetSidebarHiddenForTests } from '@/lib/shared/sidebarHidden'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  RouterLink: { template: '<a><slot /></a>' },
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    authApi: { logout: vi.fn().mockResolvedValue(undefined) },
  }
})

import AppSidebar from './AppSidebar.vue'

function mountSidebar() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...shell } },
  })
  return mount(AppSidebar, {
    global: {
      plugins: [i18n],
      stubs: {
        BrandLogo: { template: '<div data-testid="brand" />' },
        AppSidebarNav: { template: '<nav data-testid="nav" />' },
        Icon: true,
      },
    },
  })
}

describe('AppSidebar', () => {
  beforeEach(() => {
    __resetSidebarHiddenForTests()
  })
  afterEach(() => {
    __resetSidebarHiddenForTests()
  })

  it('renders brand and nav stubs', () => {
    const wrapper = mountSidebar()
    expect(wrapper.find('[data-testid="brand"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="nav"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('puts a 44px hide-nav control on the brand row (g2.1)', () => {
    const wrapper = mountSidebar()
    const row = wrapper.find('.h-14')
    expect(row.classes()).toContain('justify-between')
    const hide = wrapper.find('[data-testid="desktop-nav-hide"]')
    expect(hide.exists()).toBe(true)
    expect(hide.classes()).toContain('h-11')
    expect(hide.classes()).toContain('w-11')
    expect(hide.attributes('aria-label')).toBe('隐藏导航')
    expect(hide.attributes('aria-expanded')).toBe('true')
    expect(hide.attributes('aria-controls')).toBe('app-desktop-sidebar')
    wrapper.unmount()
  })

  it('hides with v-show so nav stays mounted and 232px is not displayed (g2.2)', async () => {
    const wrapper = mountSidebar()
    await wrapper.find('[data-testid="desktop-nav-hide"]').trigger('click')
    const aside = wrapper.find('[data-testid="app-desktop-sidebar"]')
    expect((aside.element as HTMLElement).style.display).toBe('none')
    expect(wrapper.find('[data-testid="nav"]').exists()).toBe(true)
    expect(aside.classes()).toContain('w-[232px]')
    wrapper.unmount()
  })
})

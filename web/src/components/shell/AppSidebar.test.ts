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
        ShellChromeControls: { template: '<div data-testid="shell-chrome-controls" />' },
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

  it('renders brand, nav stubs, and chrome controls (g1.2)', () => {
    const wrapper = mountSidebar()
    expect(wrapper.find('[data-testid="brand"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="nav"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="shell-chrome-controls"]').exists()).toBe(true)
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

  it('collapses to width 0 with nav still mounted (g2.2 / g3.1 / g3.3)', async () => {
    const wrapper = mountSidebar()
    await wrapper.find('[data-testid="desktop-nav-hide"]').trigger('click')
    const aside = wrapper.find('[data-testid="app-desktop-sidebar"]')
    expect((aside.element as HTMLElement).style.display).not.toBe('none')
    expect(aside.classes()).toContain('w-0')
    expect(aside.classes()).toContain('border-r-0')
    expect(aside.classes()).not.toContain('w-[232px]')
    expect(aside.classes()).not.toContain('border-r')
    expect(aside.classes()).toContain('md:flex')
    expect(aside.classes()).toContain('hidden')
    expect(aside.classes()).toContain('overflow-hidden')
    expect(aside.attributes('aria-hidden')).toBe('true')
    expect(wrapper.find('[data-testid="nav"]').exists()).toBe(true)
    expect(wrapper.html()).toContain('min-w-[232px]')
    wrapper.unmount()
  })

  it('user row uses circular avatar and borderless logout (g3.1 / g3.2 / g3.3)', () => {
    const wrapper = mountSidebar()
    const avatar = wrapper.find('[data-testid="sidebar-user-avatar"]')
    expect(avatar.exists()).toBe(true)
    expect(avatar.classes()).toContain('rounded-full')
    expect(avatar.classes()).toContain('bg-accent-dim')
    expect(avatar.classes()).toContain('h-7')
    const logout = wrapper.find('[data-testid="sidebar-logout"]')
    expect(logout.exists()).toBe(true)
    expect(logout.classes()).toContain('border-0')
    expect(logout.classes()).not.toContain('border-line')
    expect(logout.text()).toBe('登出')
    // footer stack uses unified gap-2.5 (10px) between chrome and user row
    const footer = wrapper.find('.mt-auto')
    expect(footer.classes()).toContain('gap-2.5')
    expect(footer.classes()).toContain('flex-col')
    wrapper.unmount()
  })
})

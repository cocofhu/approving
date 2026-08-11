// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/runs', meta: {} }),
  RouterLink: {
    props: ['to'],
    template:
      '<a :href="to" :data-to="to" @click="$emit(\'click\')"><slot /></a>',
  },
}))

const gateMocks = vi.hoisted(() => ({
  peek: vi.fn(),
  refresh: vi.fn(),
  count: { value: 2 },
}))

const notifMocks = vi.hoisted(() => ({
  unreadCount: { value: 0 },
}))

vi.mock('@/lib/usePendingGates', () => ({
  usePendingGates: () => ({
    count: gateMocks.count,
    peek: gateMocks.peek,
    refresh: gateMocks.refresh,
  }),
}))

vi.mock('@/lib/useRunTerminalNotifications', () => ({
  useRunTerminalNotifications: () => ({
    unreadCount: notifMocks.unreadCount,
  }),
}))

import AppSidebarNav from './AppSidebarNav.vue'

beforeEach(() => {
  vi.clearAllMocks()
  gateMocks.peek.mockResolvedValue(undefined)
  gateMocks.refresh.mockResolvedValue(undefined)
  gateMocks.count.value = 2
  notifMocks.unreadCount.value = 0
  vi.useFakeTimers()
})

function mountNav() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AppSidebarNav, {
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('AppSidebarNav', () => {
  it('renders nav links from sidebar config', async () => {
    const wrapper = mountNav()
    await flushPromises()
    expect(wrapper.find('nav').exists()).toBe(true)
    expect(gateMocks.refresh).toHaveBeenCalled()
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('shows /gates badge from pending gates and hides /notifications badge when unread is 0', async () => {
    notifMocks.unreadCount.value = 0
    const wrapper = mountNav()
    await flushPromises()
    expect(wrapper.find('[data-testid="nav-gates-badge"]').text()).toBe('2')
    expect(wrapper.find('[data-testid="nav-notifications-badge"]').exists()).toBe(false)
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('shows /notifications unread badge from the same unreadCount as topbar', async () => {
    notifMocks.unreadCount.value = 46
    const wrapper = mountNav()
    await flushPromises()
    const badge = wrapper.find('[data-testid="nav-notifications-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('46')
    // Gates badge remains independent.
    expect(wrapper.find('[data-testid="nav-gates-badge"]').text()).toBe('2')
    wrapper.unmount()
    vi.useRealTimers()
  })
})

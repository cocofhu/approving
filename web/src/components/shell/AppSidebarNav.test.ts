// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import nav from '@/locales/zh-CN/nav.json'

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

const favMocks = vi.hoisted(() => {
  // Plain { value } bags — script paths must read .value (same as gate/notif mocks).
  return {
    displayItems: {
      value: [] as Array<{
        workflowId: string
        favoritedAt: number
        name: string
        projectId: string
        projectName: string
        status: 'draft' | 'published'
      }>,
    },
    hydrateDisplay: vi.fn(async () => undefined),
    unfavorite: vi.fn(),
    getFavoriteWorkflow: vi.fn(),
  }
})

const launchMocks = vi.hoisted(() => ({
  openLaunch: vi.fn(),
}))

vi.mock('@/lib/inbox/usePendingGates', () => ({
  usePendingGates: () => ({
    count: gateMocks.count,
    peek: gateMocks.peek,
    refresh: gateMocks.refresh,
  }),
}))

vi.mock('@/lib/run/useRunTerminalNotifications', () => ({
  useRunTerminalNotifications: () => ({
    unreadCount: notifMocks.unreadCount,
  }),
}))

vi.mock('@/lib/run/useWorkflowFavorites', async () => {
  const { computed } = await import('vue')
  return {
    useWorkflowFavorites: () => ({
      // Expose a computed so template auto-tracks the hoisted bag.
      displayItems: computed(() => favMocks.displayItems.value),
      hydrateDisplay: favMocks.hydrateDisplay,
      unfavorite: favMocks.unfavorite,
      getFavoriteWorkflow: favMocks.getFavoriteWorkflow,
    }),
  }
})

vi.mock('@/lib/run/useWorkflowRunLaunch', () => ({
  useWorkflowRunLaunch: () => ({
    openLaunch: launchMocks.openLaunch,
  }),
}))

import AppSidebarNav from './AppSidebarNav.vue'

beforeEach(() => {
  vi.clearAllMocks()
  gateMocks.peek.mockResolvedValue(undefined)
  gateMocks.refresh.mockResolvedValue(undefined)
  gateMocks.count.value = 2
  notifMocks.unreadCount.value = 0
  favMocks.displayItems.value = []
  favMocks.hydrateDisplay.mockResolvedValue(undefined)
  vi.useFakeTimers()
})

function mountNav() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...nav } },
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

  it('renders independent quick-pipelines section with empty guidance', async () => {
    const wrapper = mountNav()
    await flushPromises()
    expect(wrapper.find('[data-testid="nav-quick-pipelines"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="nav-quick-pipelines-empty"]').text()).toContain('星标')
    // Primary nav still present
    expect(wrapper.find('[data-to="/notifications"]').exists()).toBe(true)
    expect(wrapper.find('[data-to="/settings"]').exists()).toBe(true)
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('lists favorites and unfavorite does not open launch', async () => {
    favMocks.displayItems.value = [
      {
        workflowId: 'wf-1',
        favoritedAt: 2,
        name: '夜间回归',
        projectId: 'p1',
        projectName: 'checkout-service',
        status: 'draft',
      },
    ]
    favMocks.getFavoriteWorkflow.mockResolvedValue({ id: 'wf-1', name: '夜间回归', nodes: [], edges: [] })
    const wrapper = mountNav()
    await flushPromises()
    expect(wrapper.find('[data-testid="nav-quick-pipelines-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="nav-quick-pipeline-item"]').text()).toContain('夜间回归')
    expect(wrapper.text()).toContain('草稿')

    await wrapper.find('[data-testid="nav-quick-pipeline-unfavorite"]').trigger('click')
    expect(favMocks.unfavorite).toHaveBeenCalledWith('wf-1', { name: '夜间回归' })
    expect(launchMocks.openLaunch).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="nav-quick-pipeline-item"]').trigger('click')
    await flushPromises()
    expect(favMocks.getFavoriteWorkflow).toHaveBeenCalledWith('wf-1')
    expect(launchMocks.openLaunch).toHaveBeenCalled()
    wrapper.unmount()
    vi.useRealTimers()
  })
})

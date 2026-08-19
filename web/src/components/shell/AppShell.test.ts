// @vitest-environment happy-dom
import { defineComponent, nextTick, reactive } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import { __resetSidebarHiddenForTests, setSidebarHidden } from '@/lib/shared/sidebarHidden'

const routeState = reactive({ path: '/', meta: {} as Record<string, unknown> })

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/lib/composables/useShutdownState', () => ({
  drainToast: { visible: false, text: '' },
  formatGrace: () => '',
  isDraining: () => false,
  isOffline: () => false,
  shutdownState: { mode: 'normal', graceRemainingSeconds: 0, message: '', checked: true },
  startShutdownPolling: vi.fn(),
  stopShutdownPolling: vi.fn(),
}))

vi.mock('@/lib/run/useWorkflowRunLaunch', async () => {
  const { ref } = await import('vue')
  return {
    useWorkflowRunLaunch: () => ({
      open: ref(false),
      target: ref(null),
      runFields: ref([]),
      runInputs: ref({}),
      runImages: ref({}),
      draftRestored: ref(false),
      openLaunch: vi.fn(),
      closeLaunch: vi.fn(),
      saveRunDraftClick: vi.fn(),
      onStarted: vi.fn(),
    }),
  }
})

import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import AppShell from './AppShell.vue'

function mountShell(opts?: { stubSidebar?: boolean; stubTopbar?: boolean }) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages, ...shell } },
  })
  return mount(AppShell, {
    slots: { default: '<div data-testid="main">内容</div>' },
    global: {
      plugins: [i18n],
      stubs: {
        AppSidebar: opts?.stubSidebar === false
          ? false
          : defineComponent({
              template:
                '<aside data-testid="sidebar" class="w-[232px] shrink-0" />',
            }),
        AppTopbar: opts?.stubTopbar === false
          ? false
          : defineComponent({
              template: '<header data-testid="topbar" />',
              emits: ['toggle-menu'],
            }),
        AppSidebarNav: { template: '<nav data-testid="shell-nav" />' },
        BrandLogo: true,
        Icon: true,
        RunLaunchModal: true,
        ServiceCommitBadge: true,
      },
    },
  })
}

describe('AppShell', () => {
  beforeEach(() => {
    __resetSidebarHiddenForTests()
    routeState.path = '/'
    routeState.meta = {}
  })
  afterEach(() => {
    __resetSidebarHiddenForTests()
  })

  it('renders sidebar and topbar with default slot', () => {
    const wrapper = mountShell()
    expect(wrapper.find('[data-testid="sidebar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="topbar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="main"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="app-refresh-bar"]').exists()).toBe(false)
    expect(wrapper.find('[aria-live="polite"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not show edge open on pages with topbar (g3.1 / g4.1)', () => {
    setSidebarHidden(true)
    const wrapper = mountShell()
    expect(wrapper.find('[data-testid="desktop-nav-edge-open"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="topbar"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows 44px edge open and pl-11 on full pages when hidden (g4.1 / g4.2)', () => {
    routeState.meta = { full: true }
    setSidebarHidden(true)
    const wrapper = mountShell()
    expect(wrapper.find('[data-testid="topbar"]').exists()).toBe(false)
    const edge = wrapper.find('[data-testid="desktop-nav-edge-open"]')
    expect(edge.exists()).toBe(true)
    expect(edge.classes()).toContain('h-11')
    expect(edge.classes()).toContain('w-11')
    expect(edge.classes()).toContain('z-20')
    expect(wrapper.find('[data-testid="app-full-main"]').classes()).toContain('pl-11')
    wrapper.unmount()
  })

  it('drops the edge button when leaving a full page while still hidden (g4.1)', async () => {
    routeState.meta = { full: true }
    setSidebarHidden(true)
    const wrapper = mountShell()
    expect(wrapper.find('[data-testid="desktop-nav-edge-open"]').exists()).toBe(true)
    routeState.meta = {}
    await nextTick()
    expect(wrapper.find('[data-testid="desktop-nav-edge-open"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="topbar"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps hidden preference across route changes (g1.2 / g5.2)', async () => {
    setSidebarHidden(true)
    const wrapper = mountShell()
    routeState.path = '/projects'
    await nextTick()
    expect(wrapper.find('[data-testid="desktop-nav-edge-open"]').exists()).toBe(false)
    routeState.meta = { full: true }
    routeState.path = '/runs/abc'
    await nextTick()
    expect(wrapper.find('[data-testid="desktop-nav-edge-open"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps height chain classes (g5.2 fill lock)', () => {
    const wrapper = mountShell()
    expect(wrapper.find('.h-screen').exists()).toBe(true)
    const main = wrapper.find('main')
    expect(main.classes()).toContain('min-h-0')
    expect(main.classes()).toContain('flex-1')
    wrapper.unmount()
  })

  it('mobile drawer stays md:hidden (g3.2 source lock)', () => {
    const src = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), 'AppShell.vue'),
      'utf8',
    )
    expect(src).toMatch(/bg-black\/50 md:hidden/)
    expect(src).toMatch(/shadow-drawer md:hidden/)
  })

  it('desktop hide uses v-show not a mobile drawer (g2.2 / g3.2)', async () => {
    const wrapper = mountShell({ stubSidebar: false })
    const aside = wrapper.find('[data-testid="app-desktop-sidebar"]')
    expect(aside.exists()).toBe(true)
    await wrapper.find('[data-testid="desktop-nav-hide"]').trigger('click')
    expect((aside.element as HTMLElement).style.display).toBe('none')
    expect(wrapper.find('[data-testid="shell-nav"]').exists()).toBe(true)
    expect(aside.classes()).toContain('hidden')
    expect(aside.classes()).toContain('md:flex')
    wrapper.unmount()
  })
})

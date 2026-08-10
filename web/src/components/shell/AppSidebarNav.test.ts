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
    template: '<a @click="$emit(\'click\')"><slot /></a>',
  },
}))

const gateMocks = vi.hoisted(() => ({
  peek: vi.fn(),
  refresh: vi.fn(),
}))

vi.mock('@/lib/inbox/usePendingGates', () => ({
  usePendingGates: () => ({
    count: { value: 2 },
    peek: gateMocks.peek,
    refresh: gateMocks.refresh,
  }),
}))

import AppSidebarNav from './AppSidebarNav.vue'

beforeEach(() => {
  vi.clearAllMocks()
  gateMocks.peek.mockResolvedValue(undefined)
  gateMocks.refresh.mockResolvedValue(undefined)
  vi.useFakeTimers()
})

describe('AppSidebarNav', () => {
  it('renders nav links from sidebar config', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(AppSidebarNav, {
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
    await flushPromises()
    expect(wrapper.find('nav').exists()).toBe(true)
    expect(gateMocks.refresh).toHaveBeenCalled()
    wrapper.unmount()
    vi.useRealTimers()
  })
})

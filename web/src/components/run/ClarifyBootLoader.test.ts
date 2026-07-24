// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ClarifyBootLoader from './ClarifyBootLoader.vue'

function mountLoader(phase: 'pending' | 'starting') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ClarifyBootLoader, {
    props: { phase },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ClarifyBootLoader', () => {
  it('renders pending phase', () => {
    const wrapper = mountLoader('pending')
    expect(wrapper.text()).toMatch(/等待|队列/)
    wrapper.unmount()
  })

  it('renders starting phase with cycling steps', async () => {
    vi.useFakeTimers()
    const wrapper = mountLoader('starting')
    expect(wrapper.text()).toMatch(/沙箱|ACP|问题/)
    vi.advanceTimersByTime(2700)
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('span.rounded-full, span.h-1\\.5').length).toBeGreaterThan(0)
    vi.useRealTimers()
    wrapper.unmount()
  })
})

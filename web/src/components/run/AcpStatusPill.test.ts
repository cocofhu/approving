// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import AcpStatusPill from './AcpStatusPill.vue'

function mountPill(props: { busy?: boolean; connected?: boolean } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(AcpStatusPill, {
    props,
    global: { plugins: [i18n] },
  })
}

describe('AcpStatusPill', () => {
  it('shows busy state', () => {
    const wrapper = mountPill({ busy: true, connected: true })
    expect(wrapper.text()).toMatch(/运行中|busy/i)
    wrapper.unmount()
  })

  it('shows idle when connected but not busy', () => {
    const wrapper = mountPill({ busy: false, connected: true })
    expect(wrapper.text()).toMatch(/空闲|idle/i)
    wrapper.unmount()
  })

  it('shows disconnected when not connected', () => {
    const wrapper = mountPill({ busy: false, connected: false })
    expect(wrapper.text()).toMatch(/未连接|断开|disconnected/i)
    wrapper.unmount()
  })
})

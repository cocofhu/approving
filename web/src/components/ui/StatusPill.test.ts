// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import StatusPill from './StatusPill.vue'

function mountPill(status: string) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(StatusPill, {
    props: { status, size: 'sm' },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('StatusPill', () => {
  it('renders localized running label', () => {
    const wrapper = mountPill('running')
    expect(wrapper.text().length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('falls back for unknown status', () => {
    const wrapper = mountPill('unknown_xyz')
    expect(wrapper.find('span').exists()).toBe(true)
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { WFEdge, WFNode } from '@/lib/types'
import NodePalette from './NodePalette.vue'

function mountPalette() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(NodePalette, {
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

describe('NodePalette', () => {
  it('renders searchable palette groups', () => {
    const wrapper = mountPalette()
    expect(wrapper.find('input').exists()).toBe(true)
    expect(wrapper.text().length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('filters nodes when search query is entered', async () => {
    const wrapper = mountPalette()
    const before = wrapper.text()
    await wrapper.find('input').setValue('xyz-not-found-abc')
    expect(wrapper.text()).not.toBe(before)
    wrapper.unmount()
  })
})

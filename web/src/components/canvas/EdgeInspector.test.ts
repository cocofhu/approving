// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { WFEdge } from '@/lib/shared/types'
import EdgeInspector from './EdgeInspector.vue'

function sampleEdge(): WFEdge {
  return {
    id: 'e1',
    source: 'n1',
    target: 'n2',
    kind: 'success',
    carry: ['foo'],
  }
}

function mountInspector(edge = sampleEdge()) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(EdgeInspector, {
    props: { edge },
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

describe('EdgeInspector', () => {
  it('shows edge endpoints and transition kinds', () => {
    const wrapper = mountInspector()
    expect(wrapper.text()).toContain('n1 → n2')
    expect(wrapper.text()).toMatch(/成功|success/i)
    wrapper.unmount()
  })

  it('emits delete when trash button is clicked', async () => {
    const wrapper = mountInspector()
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('delete')).toHaveLength(1)
    wrapper.unmount()
  })

  it('updates edge kind via radio selection', async () => {
    const edge = sampleEdge()
    const wrapper = mountInspector(edge)
    const radios = wrapper.findAll('input[type="radio"]')
    expect(radios.length).toBeGreaterThan(0)
    await radios[1]!.setValue('failure')
    expect(edge.kind).toBe('failure')
    wrapper.unmount()
  })
})

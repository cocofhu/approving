// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import CompositeVarBlock from './CompositeVarBlock.vue'

function mountBlock(value: unknown) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(CompositeVarBlock, {
    props: { value },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('CompositeVarBlock', () => {
  it('renders text via VarValueDisplay', () => {
    const wrapper = mountBlock('hello world')
    expect(wrapper.text()).toContain('hello world')
    wrapper.unmount()
  })

  it('shows image strip for composite with images', () => {
    const wrapper = mountBlock({
      text: 'pic',
      images: [{ mime: 'image/png', data: 'x', name: 'x.png' }],
    })
    expect(wrapper.findAll('img').length).toBe(1)
    wrapper.unmount()
  })
})

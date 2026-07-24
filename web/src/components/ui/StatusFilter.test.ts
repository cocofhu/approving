// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import StatusFilter from './StatusFilter.vue'

function mountFilter(modelValue: string[] = []) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(StatusFilter, {
    props: { modelValue },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('StatusFilter', () => {
  it('shows all statuses label when nothing selected', () => {
    const wrapper = mountFilter([])
    expect(wrapper.text()).toMatch(/全部|all/i)
    wrapper.unmount()
  })

  it('opens dropdown and toggles status', async () => {
    const wrapper = mountFilter([])
    await wrapper.find('button').trigger('click')
    const options = wrapper.findAll('.card button')
    expect(options.length).toBeGreaterThan(0)
    await options[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import LangSelect from './LangSelect.vue'

function mountLang(modelValue: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(LangSelect, {
    props: { modelValue },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('LangSelect', () => {
  it('shows current locale label', () => {
    const wrapper = mountLang('zh-CN')
    expect(wrapper.text()).toContain('中文')
    wrapper.unmount()
  })

  it('emits locale change when option chosen', async () => {
    const wrapper = mountLang('zh-CN')
    await wrapper.find('button').trigger('click')
    const options = wrapper.findAll('[role="option"], button')
    const en = options.find((o) => o.text().includes('English'))
    expect(en).toBeTruthy()
    await en!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['en'])
    wrapper.unmount()
  })
})

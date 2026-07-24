// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import ParagraphInput from './ParagraphInput.vue'

function mountInput(textOnly = false) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(ParagraphInput, {
    props: { text: 'hello', textOnly },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ParagraphInput', () => {
  it('renders textarea with initial text', () => {
    const wrapper = mountInput(true)
    const ta = wrapper.find('textarea')
    expect(ta.exists()).toBe(true)
    expect((ta.element as HTMLTextAreaElement).value).toBe('hello')
    wrapper.unmount()
  })

  it('updates text model on input', async () => {
    const wrapper = mountInput(true)
    await wrapper.find('textarea').setValue('updated')
    expect(wrapper.emitted('update:text')?.[0]).toEqual(['updated'])
    wrapper.unmount()
  })

  it('shows attach control when not text-only', () => {
    const wrapper = mountInput(false)
    expect(wrapper.find('[data-testid="paragraph-input-attach"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="paragraph-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="paragraph-input-root"]').attributes('data-text-only')).toBe('0')
    wrapper.unmount()
  })

  it('hides attach control when text-only', () => {
    const wrapper = mountInput(true)
    expect(wrapper.find('[data-testid="paragraph-input-attach"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="paragraph-input-root"]').attributes('data-text-only')).toBe('1')
    wrapper.unmount()
  })
})

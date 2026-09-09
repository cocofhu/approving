// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import ReactConnectingState from './ReactConnectingState.vue'

function mountSidebar(locale: 'zh-CN' | 'en') {
  const i18n = locale === 'zh-CN'
    ? createI18n({
        legacy: false,
        locale,
        messages: { 'zh-CN': { ...common, ...pages } },
      })
    : createI18n({
        legacy: false,
        locale,
        messages: { en: { ...enCommon, ...enPages } },
      })
  return mount(ReactConnectingState, {
    global: {
      plugins: [i18n],
      stubs: { Icon: true },
    },
  })
}

describe('ReactConnectingState', () => {
  it('shows the Chinese connecting state with all actions disabled', () => {
    const wrapper = mountSidebar('zh-CN')
    expect(wrapper.get('[data-testid="react-connecting-pill"]').text()).toContain('连接中')
    const input = wrapper.get('[data-testid="react-connecting-input"]')
    expect(input.attributes('placeholder')).toBe('连接中，暂不可输入')
    expect((input.element as HTMLTextAreaElement).disabled).toBe(true)
    expect((wrapper.get('[data-testid="react-connecting-send"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.get('[data-testid="react-connecting-confirm"]').element as HTMLButtonElement).disabled).toBe(true)
  })

  it('shows the English connecting copy', () => {
    const wrapper = mountSidebar('en')
    expect(wrapper.get('[data-testid="react-connecting-pill"]').text()).toContain('Connecting')
    expect(wrapper.get('[data-testid="react-connecting-input"]').attributes('placeholder')).toContain('Connecting')
  })

  it('renders a stable artifact-stage skeleton', () => {
    const wrapper = mount(ReactConnectingState, {
      props: { mode: 'stage' },
      global: {
        plugins: [createI18n({
          legacy: false,
          locale: 'zh-CN',
          messages: { 'zh-CN': { ...common, ...pages } },
        })],
        stubs: { Icon: true },
      },
    })
    expect(wrapper.get('[data-testid="react-connecting-stage"]').attributes('aria-busy')).toBe('true')
    expect(wrapper.text()).toContain('产物区将在会话就绪后填充')
  })
})

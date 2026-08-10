// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import AppInlineError from './AppInlineError.vue'
import EmptyState from './EmptyState.vue'

function mountErr(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(AppInlineError, {
    props,
    global: { plugins: [i18n] },
  })
}

describe('AppInlineError', () => {
  it('is visually distinct from EmptyState and retry hit area is at least 44px', async () => {
    const wrapper = mountErr({ message: '网络错误' })
    expect(wrapper.get('[data-testid="app-inline-error"]').classes().join(' ')).toContain('border-err')
    expect(wrapper.text()).toContain('加载失败')
    expect(wrapper.text()).toContain('网络错误')
    expect(wrapper.text()).toContain('重试')
    const retry = wrapper.get('[data-testid="app-inline-error-retry"]')
    expect(retry.classes().join(' ')).toContain('min-h-[44px]')
    await retry.trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()

    const empty = mount(EmptyState, {
      props: { title: '暂无内容' },
      global: { stubs: { Icon: true } },
    })
    expect(empty.text()).toContain('暂无内容')
    expect(empty.html()).not.toContain('app-inline-error')
    expect(empty.html()).not.toContain('重试')
    wrapper.unmount()
    empty.unmount()
  })
})

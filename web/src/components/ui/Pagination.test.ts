// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import Pagination from './Pagination.vue'

function mountPager(page = 1, total = 50) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common } },
  })
  return mount(Pagination, {
    props: { page, pageSize: 10, total },
    global: { plugins: [i18n] },
  })
}

describe('Pagination', () => {
  it('shows summary and page buttons', () => {
    const wrapper = mountPager(2, 50)
    expect(wrapper.text()).toMatch(/50/)
    expect(wrapper.findAll('button').length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('emits page update when next is clicked', async () => {
    const wrapper = mountPager(1, 50)
    const next = wrapper.findAll('button').find((b) => !b.attributes('disabled'))
    const buttons = wrapper.findAll('button')
    const enabled = buttons.filter((b) => !(b.element as HTMLButtonElement).disabled)
    const last = enabled[enabled.length - 1]
    expect(last).toBeTruthy()
    await last!.trigger('click')
    expect(wrapper.emitted('update:page')).toBeTruthy()
    wrapper.unmount()
  })
})

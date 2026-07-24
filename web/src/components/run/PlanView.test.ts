// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import PlanView, { type PlanDoc } from './PlanView.vue'

function mountPlan(doc: PlanDoc, accent?: string) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(PlanView, {
    props: { doc, accent },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('PlanView', () => {
  it('renders goals and progress', () => {
    const doc: PlanDoc = {
      title: '实施计划',
      goals: [
        {
          id: 'g1',
          title: '大目标',
          status: 'in_progress',
          subgoals: [
            { id: 'g1.1', title: '子目标 A', status: 'done' },
            { id: 'g1.2', title: '子目标 B', status: 'pending' },
          ],
        },
      ],
    }
    const wrapper = mountPlan(doc)
    expect(wrapper.text()).toContain('实施计划')
    expect(wrapper.text()).toContain('大目标')
    expect(wrapper.text()).toContain('子目标 A')
    expect(wrapper.text()).toContain('1/2')
    wrapper.unmount()
  })

  it('uses default title when doc.title missing', () => {
    const wrapper = mountPlan({ goals: [{ id: 'g1', title: '仅目标', status: 'pending' }] })
    expect(wrapper.text()).toMatch(/计划|Plan/)
    expect(wrapper.text()).toContain('仅目标')
    wrapper.unmount()
  })
})

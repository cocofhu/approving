// @vitest-environment happy-dom
import { defineComponent, h } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { provideReviewAnnotate } from '@/lib/reviewAnnotate'
import TestResultView, { type TestResultDoc } from './TestResultView.vue'

function mountView(doc: TestResultDoc, withAnnotate = false) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  if (!withAnnotate) {
    return mount(TestResultView, {
      props: { doc, runId: 'run-1', artifacts: [] },
      global: { plugins: [i18n], stubs: { Icon: true, AppModal: true } },
    })
  }
  const Host = defineComponent({
    setup() {
      provideReviewAnnotate({ enabled: true, annotate: () => {} })
      return () => h(TestResultView, { doc, runId: 'run-1', artifacts: [] })
    },
  })
  return mount(Host, {
    global: { plugins: [i18n], stubs: { Icon: true, AppModal: true } },
  })
}

describe('TestResultView', () => {
  it('renders summary and case list', () => {
    const doc: TestResultDoc = {
      summary: '核心流程通过',
      assessment: '可发布',
      passed: 2,
      failed: 1,
      skipped: 0,
      cases: [
        { name: '登录流程', status: 'passed', detail: 'OK' },
        { name: '注册流程', status: 'failed', detail: '验证码错误' },
      ],
      defects: [{ title: '按钮样式', severity: 'low', detail: '颜色不一致' }],
      variances: '跳过了 E2E 慢测',
    }
    const wrapper = mountView(doc)
    expect(wrapper.text()).toContain('核心流程通过')
    expect(wrapper.text()).toContain('登录流程')
    expect(wrapper.text()).toContain('注册流程')
    expect(wrapper.text()).toContain('按钮样式')
    expect(wrapper.text()).toContain('可发布')
    wrapper.unmount()
  })

  it('shows empty sections gracefully for minimal doc', () => {
    const wrapper = mountView({ summary: '无用例' })
    expect(wrapper.text()).toContain('无用例')
    wrapper.unmount()
  })

  it('exposes AnnotateBtn for variances', () => {
    const wrapper = mountView(
      { summary: '有偏差', variances: '跳过了 E2E 慢测' },
      true,
    )
    const titles = wrapper.findAll('button').map((b) => b.attributes('title') || '')
    expect(titles.some((t) => t.includes('variances'))).toBe(true)
    wrapper.unmount()
  })
})

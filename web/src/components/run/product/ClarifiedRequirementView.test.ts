// @vitest-environment happy-dom
import { defineComponent, h } from 'vue'
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { provideReviewAnnotate } from '@/lib/reviewAnnotate'
import ClarifiedRequirementView, { type ClarifiedRequirementDoc } from './ClarifiedRequirementView.vue'

function mountView(doc: ClarifiedRequirementDoc, withAnnotate = false) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  if (!withAnnotate) {
    return mount(ClarifiedRequirementView, {
      props: { doc, accent: '#818CF8' },
      global: { plugins: [i18n], stubs: { Icon: true } },
    })
  }
  const Host = defineComponent({
    setup() {
      provideReviewAnnotate({ enabled: true, annotate: () => {} })
      return () => h(ClarifiedRequirementView, { doc, accent: '#818CF8' })
    },
  })
  return mount(Host, {
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('ClarifiedRequirementView', () => {
  it('renders summary and functional requirements', () => {
    const doc: ClarifiedRequirementDoc = {
      title: '用户登录',
      summary: '支持 OIDC 登录',
      background: '现有系统无 SSO',
      goals: ['提升登录体验'],
      functional_requirements: [
        {
          title: 'OIDC 登录',
          detail: '接入 Synology SSO',
          priority: 'must',
          acceptance_criteria: ['可完成登录跳转'],
        },
      ],
      in_scope: ['Web 登录'],
      out_of_scope: ['移动端'],
    }
    const wrapper = mountView(doc)
    expect(wrapper.text()).toContain('用户登录')
    expect(wrapper.text()).toContain('支持 OIDC 登录')
    expect(wrapper.text()).toContain('OIDC 登录')
    expect(wrapper.text()).toContain('Web 登录')
    wrapper.unmount()
  })

  it('renders personas and scenarios when present', () => {
    const doc: ClarifiedRequirementDoc = {
      summary: '摘要',
      personas: [{ name: '开发者', description: '日常使用者', goals: ['快速部署'] }],
      user_scenarios: [{ name: '部署应用', actor: '开发者', flow: '提交 MR', outcome: '自动预览' }],
    }
    const wrapper = mountView(doc)
    expect(wrapper.text()).toContain('开发者')
    expect(wrapper.text()).toContain('部署应用')
    wrapper.unmount()
  })

  it('renders secondary list fields', () => {
    const doc: ClarifiedRequirementDoc = {
      summary: '摘要',
      assumptions: ['用户已登录'],
      dependencies: ['OIDC Provider'],
      constraints: ['不改后端协议'],
      risks: [{ id: 'r1', description: '会话过期', mitigation: '刷新令牌' }],
      glossary: [{ term: 'Gate', definition: '人工审批节点' }],
    }
    const wrapper = mountView(doc)
    expect(wrapper.text()).toContain('用户已登录')
    expect(wrapper.text()).toContain('OIDC Provider')
    expect(wrapper.text()).toContain('不改后端协议')
    expect(wrapper.text()).toContain('会话过期')
    expect(wrapper.text()).toContain('Gate')
    wrapper.unmount()
  })

  it('exposes AnnotateBtn for personas/interfaces/entities/business_rules', () => {
    const doc: ClarifiedRequirementDoc = {
      summary: '摘要',
      personas: [{ id: 'u1', name: '开发者' }],
      external_interfaces: [{ id: 'i1', name: 'OIDC' }],
      data_entities: [{ id: 'e1', name: 'User' }],
      business_rules: ['仅管理员可审批'],
    }
    const wrapper = mountView(doc, true)
    const titles = wrapper.findAll('button').map((b) => b.attributes('title') || '')
    expect(titles.some((t) => t.includes('personas[u1]'))).toBe(true)
    expect(titles.some((t) => t.includes('external_interfaces[i1]'))).toBe(true)
    expect(titles.some((t) => t.includes('data_entities[e1]'))).toBe(true)
    expect(titles.some((t) => t.includes('business_rules[0]'))).toBe(true)
    wrapper.unmount()
  })
})

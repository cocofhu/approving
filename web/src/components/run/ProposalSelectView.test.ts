// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import ProposalSelectView, { type ProposalsDoc } from './ProposalSelectView.vue'

function mountView(doc: ProposalsDoc, opts: { resolvedId?: string; disabled?: boolean; readonly?: boolean } = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ProposalSelectView, {
    props: {
      doc,
      resolvedId: opts.resolvedId ?? null,
      disabled: opts.disabled ?? false,
      readonly: opts.readonly ?? false,
    },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

const doc: ProposalsDoc = {
  context: '需要选择架构方案',
  decision_drivers: ['可维护性', '成本'],
  proposals: [
    {
      id: 'p1',
      title: '方案 A',
      summary: '单体架构',
      recommended: true,
      effort: 'low',
      risk: 'low',
      pros: ['简单'],
      cons: ['扩展性'],
    },
    {
      id: 'p2',
      title: '方案 B',
      summary: '微服务',
      effort: 'high',
      risk: 'medium',
    },
  ],
}

describe('ProposalSelectView', () => {
  it('renders context and recommended proposal', () => {
    const wrapper = mountView(doc)
    expect(wrapper.text()).toContain('需要选择架构方案')
    expect(wrapper.text()).toContain('方案 A')
    expect(wrapper.text()).toContain('1 / 2')
    expect(wrapper.text()).toContain('选择方案')
    expect(wrapper.text()).toContain('共 2 个候选')
    wrapper.unmount()
  })

  it('navigates to next proposal', async () => {
    const wrapper = mountView(doc)
    const nextBtn = wrapper.findAll('button').find((b) => b.attributes('title')?.includes('下一') || b.text().includes('›'))
    expect(nextBtn).toBeTruthy()
    await nextBtn!.trigger('click')
    expect(wrapper.text()).toContain('方案 B')
    expect(wrapper.text()).toContain('2 / 2')
    wrapper.unmount()
  })

  it('emits select with proposal id', async () => {
    const wrapper = mountView(doc)
    const selectBtn = wrapper.findAll('button').find((b) => b.text().includes('选择') || b.text().includes('确认'))
    expect(selectBtn).toBeTruthy()
    await selectBtn!.trigger('click')
    expect(wrapper.emitted('select')).toEqual([['p1']])
    wrapper.unmount()
  })

  it('does not emit when disabled or already resolved', async () => {
    const disabled = mountView(doc, { disabled: true })
    const btn = disabled.findAll('button').find((b) => b.text().includes('选择'))
    if (btn) await btn.trigger('click')
    expect(disabled.emitted('select')).toBeFalsy()
    disabled.unmount()

    const resolved = mountView(doc, { resolvedId: 'p1' })
    const btn2 = resolved.findAll('button').find((b) => b.text().includes('选择'))
    if (btn2) await btn2.trigger('click')
    expect(resolved.emitted('select')).toBeFalsy()
    resolved.unmount()
  })

  it('renders single candidate as readonly info without pick CTA', () => {
    const single: ProposalsDoc = {
      context: '方向已明确',
      proposals: [{ id: 'p1', title: '唯一方案', recommended: true }],
    }
    const wrapper = mountView(single)
    expect(wrapper.text()).toContain('方案信息')
    expect(wrapper.text()).toContain('唯一候选 · 无需点选')
    expect(wrapper.text()).not.toContain('共 1 个候选')
    expect(wrapper.text()).not.toContain('选择一个作为最终方案')
    const pick = wrapper.findAll('button').find((b) => b.text().includes('选择此方案'))
    expect(pick).toBeFalsy()
    wrapper.unmount()
  })
})

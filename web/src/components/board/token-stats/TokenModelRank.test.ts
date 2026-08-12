// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import TokenModelRank from './TokenModelRank.vue'
import { colorForModel } from './tokenModelColors'

const i18nZh = () =>
  createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })

const i18nEn = () =>
  createI18n({
    legacy: false,
    locale: 'en',
    messages: { en: { ...enCommon, ...enPages } },
  })

function expectNoFilledTagCopy(text: string) {
  expect(text).not.toContain('含补全')
  expect(text).not.toContain('includes filled data')
}

const rankModels = [
  { modelKey: 'cursor-grok-4.5-high-fast', name: 'cursor-grok-4.5-high-fast', total: 800, filled: true },
  { modelKey: 'gpt-5.6-sol-medium', name: 'gpt-5.6-sol-medium', total: 600, filled: true },
  { modelKey: '未知/未分桶', name: '未知/未分桶', total: 400, unknown: true },
  { modelKey: 'other', name: 'other', total: 200, other: true },
]

describe('TokenModelRank hides filledTag (g2.2)', () => {
  it('zh-CN: filled/unknown/other rows have no 含补全; data-filled keeps #34D399 / #71717A', () => {
    expect(colorForModel(rankModels[0]!, 0)).toBe('#34D399')
    expect(colorForModel(rankModels[2]!, 2)).toBe('#71717A')
    expect(colorForModel(rankModels[3]!, 3)).toBe('#A1A1AA')

    const wrapper = mount(TokenModelRank, {
      props: { models: rankModels },
      global: { plugins: [i18nZh()] },
    })
    const root = wrapper.find('[data-testid="token-model-rank"]')
    expect(root.exists()).toBe(true)
    expectNoFilledTagCopy(root.text())
    expect(wrapper.html()).not.toContain('含补全')

    const filledRows = wrapper.findAll('[data-filled="1"]')
    expect(filledRows).toHaveLength(2)
    for (const row of filledRows) {
      expectNoFilledTagCopy(row.text())
      expect(row.text()).not.toContain('含补全')
      const bar = row.find('.h-full')
      expect(bar.attributes('style')).toMatch(/background:\s*#34D399/i)
      expect(row.find('.text-ok').exists()).toBe(true)
    }

    const unknownRow = wrapper.find('[data-unknown="1"]')
    expect(unknownRow.exists()).toBe(true)
    expect(unknownRow.attributes('data-filled')).toBe('0')
    expect(unknownRow.text()).toContain('未知/未分桶')
    expectNoFilledTagCopy(unknownRow.text())
    expect(unknownRow.find('.h-full').attributes('style')).toMatch(/background:\s*#71717A/i)

    const otherRow = wrapper.find('[data-other="1"]')
    expect(otherRow.exists()).toBe(true)
    expect(otherRow.text()).toContain('other（其余模型）')
    expectNoFilledTagCopy(otherRow.text())
    expect(otherRow.find('.h-full').attributes('style')).toMatch(/background:\s*#A1A1AA/i)

    expect(root.text()).toContain('cursor-grok-4.5-high-fast')
    expect(root.text()).toContain('gpt-5.6-sol-medium')
    wrapper.unmount()
  })

  it('en locale: no includes filled data; other bucket stays unlabeled; colors unchanged', () => {
    const wrapper = mount(TokenModelRank, {
      props: { models: rankModels },
      global: { plugins: [i18nEn()] },
    })
    const root = wrapper.find('[data-testid="token-model-rank"]')
    expectNoFilledTagCopy(root.text())
    expect(wrapper.html()).not.toContain('includes filled data')
    expect(wrapper.html()).not.toContain('含补全')

    const filledRows = wrapper.findAll('[data-filled="1"]')
    expect(filledRows).toHaveLength(2)
    for (const row of filledRows) {
      expectNoFilledTagCopy(row.text())
      expect(row.find('.h-full').attributes('style')).toMatch(/background:\s*#34D399/i)
    }

    const unknownRow = wrapper.find('[data-unknown="1"]')
    expect(unknownRow.find('.h-full').attributes('style')).toMatch(/background:\s*#71717A/i)

    const otherRow = wrapper.find('[data-other="1"]')
    expect(otherRow.text()).toContain('other (remaining models)')
    expectNoFilledTagCopy(otherRow.text())
    wrapper.unmount()
  })

  it('other bucket does not gain filledTag even if filled=true (g2.2 other)', () => {
    const wrapper = mount(TokenModelRank, {
      props: {
        models: [{ modelKey: 'other', name: 'other', total: 50, other: true, filled: true }],
      },
      global: { plugins: [i18nZh()] },
    })
    const otherRow = wrapper.find('[data-other="1"]')
    expect(otherRow.attributes('data-filled')).toBe('1')
    expectNoFilledTagCopy(otherRow.text())
    expect(otherRow.text()).toContain('other（其余模型）')
    wrapper.unmount()
  })
})

describe('TokenModelRank unknown vs other (g3.3)', () => {
  it('qualifying unknown keeps 未知/未分桶, data-unknown, gray text and #71717A bar', () => {
    const models = [
      { modelKey: 'claude-sonnet-4', name: 'claude-sonnet-4', total: 100 },
      { modelKey: '未知/未分桶', name: '未知/未分桶', total: 80, unknown: true },
      { name: 'other', total: 30, other: true },
    ]
    expect(colorForModel(models[1]!, 1)).toBe('#71717A')
    expect(colorForModel(models[2]!, 2)).toBe('#A1A1AA')

    const wrapper = mount(TokenModelRank, {
      props: { models },
      global: { plugins: [i18nZh()] },
    })

    expect(wrapper.html()).not.toMatch(/「未知」与 other 不同/)
    expect(wrapper.html()).not.toMatch(/Unknown is not the same as other/i)
    expect(wrapper.html()).not.toMatch(/相关用量按其实际消耗参与排行/)

    const unk = wrapper.find('[data-unknown="1"]')
    expect(unk.exists()).toBe(true)
    expect(unk.attributes('data-other')).toBe('0')
    expect(unk.text()).toContain('未知/未分桶')
    expect(unk.text()).toContain('未知')
    expect(unk.find('[data-testid="unknown-model-badge"]').exists()).toBe(true)
    expect(unk.text()).not.toContain('other（其余模型）')
    expect(unk.find('.text-txt3').exists()).toBe(true)
    expect(unk.find('.h-full').attributes('style')).toMatch(/#71717A/i)

    const other = wrapper.find('[data-other="1"]')
    expect(other.exists()).toBe(true)
    expect(other.attributes('data-unknown')).toBe('0')
    expect(other.text()).toContain('other（其余模型）')
    expect(other.find('.h-full').attributes('style')).toMatch(/#A1A1AA/i)
    wrapper.unmount()
  })

  it('shows configured alias with unknown badge; color uses unknown flag not name literal', () => {
    const models = [
      { modelKey: 'gpt-5', name: 'gpt-5', total: 100 },
      { modelKey: '未知/未分桶', name: 'gpt-5', total: 80, unknown: true },
    ]
    expect(colorForModel(models[1]!, 1)).toBe('#71717A')
    expect(colorForModel({ name: 'gpt-5', unknown: false }, 0)).not.toBe('#71717A')

    const wrapper = mount(TokenModelRank, {
      props: { models },
      global: { plugins: [i18nZh()] },
    })
    const unk = wrapper.find('[data-unknown="1"]')
    expect(unk.text()).toContain('gpt-5')
    expect(unk.find('[data-testid="unknown-model-badge"]').exists()).toBe(true)
    expect(unk.find('.h-full').attributes('style')).toMatch(/#71717A/i)
    // Two rows with same display text stay distinct via data-unknown.
    expect(wrapper.findAll('[data-testid="token-model-rank"] > li')).toHaveLength(2)
    wrapper.unmount()
  })
})

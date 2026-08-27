// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import TokenModelComposition from './TokenModelComposition.vue'
import { colorForModel } from './tokenModelColors'

vi.mock('vue-echarts', () => ({
  default: {
    name: 'VChart',
    template: '<div data-testid="mock-vchart" class="h-full w-full"><canvas /></div>',
    props: ['option'],
  },
}))

vi.mock('@/components/charts/echartsSetup', () => ({
  registerECharts: () => {},
}))

const i18n = () =>
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

/** Screenshot-like sample: unknown grey + two filled (ACP_BRIDGE backfill) buckets. */
const screenshotLikeModels = [
  { modelKey: '未知/未分桶', name: '未知模型', total: 100, unknown: true },
  { modelKey: 'cursor-grok-4.5-high-fast', name: 'cursor-grok-4.5-high-fast', total: 80, filled: true },
  { modelKey: 'gpt-5.6-sol-medium', name: 'gpt-5.6-sol-medium', total: 60, filled: true },
]

function expectNoFilledTagCopy(text: string) {
  expect(text).not.toContain('含补全')
  expect(text).not.toContain('includes filled data')
}

function chartData(vm: unknown) {
  return (vm as { chartData: { name: string; value: number; color?: string }[] }).chartData
}

describe('TokenModelComposition ECharts pie (g1.1)', () => {
  it('renders ECharts pie host without svg path sectors (g1.1/g2.1)', () => {
    const wrapper = mount(TokenModelComposition, {
      props: {
        models: [{ modelKey: '未知/未分桶', name: '未知模型', total: 1056, unknown: true }],
      },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect(pie.exists()).toBe(true)
    expect(pie.find('[data-testid="mock-vchart"]').exists()).toBe(true)
    expect(pie.find('svg').exists()).toBe(false)
    expect(pie.findAll('path').length).toBe(0)
    expect(wrapper.html()).not.toMatch(/conic-gradient/i)
    expect(wrapper.html()).not.toMatch(/rounded-full/)
    const data = chartData(wrapper.vm)
    expect(data).toHaveLength(1)
    expect(data[0]!.color).toBe('#71717A')
    wrapper.unmount()
  })

  it('unknown-only near-full circle: #71717A sector + legend 100% (g1.2/g2.2)', () => {
    const unknown = { modelKey: '未知/未分桶', name: '未知模型', total: 1056240000, unknown: true }
    expect(colorForModel(unknown, 0)).toBe('#71717A')

    const wrapper = mount(TokenModelComposition, {
      props: { models: [unknown] },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect(pie.find('[data-testid="mock-vchart"]').exists()).toBe(true)
    const data = chartData(wrapper.vm)
    expect(data[0]!.color).toBe('#71717A')
    const legend = wrapper.find('[data-testid="token-model-legend"]')
    expect(legend.text()).toContain('未知模型')
    expect(legend.text()).toContain('100%')
    expect(legend.text()).toContain('1.06B')
    wrapper.unmount()
  })

  it('thin wedge + dominant bucket: ECharts sectors keep colors (g2.2 attach-like)', () => {
    const models = [
      { modelKey: 'cursor-grok', name: 'cursor-grok-4.5-high-fast', total: 50090000, filled: true },
      { modelKey: '未知/未分桶', name: '未知模型', total: 1059710000, unknown: true },
    ]
    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    const data = chartData(wrapper.vm)
    expect(data).toHaveLength(2)
    expect(data[0]!.color).toBe(colorForModel(models[0]!, 0))
    expect(data[1]!.color).toBe('#71717A')
    const legend = wrapper.find('[data-testid="token-model-legend"]').text()
    expect(legend).toContain('4.5%')
    expect(legend).toContain('95.5%')
    expect(legend).toContain('50.09M')
    expect(legend).toContain('1.06B')
    expect(legend).toContain('cursor-grok-4.5-high-fast')
    expectNoFilledTagCopy(legend)
    expect(colorForModel(models[0]!, 0)).toBe('#34D399')
    expect(wrapper.html()).not.toMatch(/conic-gradient/i)
    wrapper.unmount()
  })

  it('multi-bucket: ECharts sectors keep unknown #71717A and other #A1A1AA (g1.3/g2.1/g2.2)', () => {
    const models = [
      { modelKey: 'claude-sonnet-4', name: 'claude-sonnet-4', total: 600, filled: true },
      { modelKey: '未知/未分桶', name: '未知模型', total: 300, unknown: true },
      { modelKey: 'other', name: 'other', total: 100, other: true },
    ]
    expect(colorForModel(models[1]!, 1)).toBe('#71717A')
    expect(colorForModel(models[2]!, 2)).toBe('#A1A1AA')

    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    const colors = chartData(wrapper.vm).map((d) => d.color)
    expect(colors).toContain('#71717A')
    expect(colors).toContain('#A1A1AA')
    const legend = wrapper.find('[data-testid="token-model-legend"]').text()
    expect(legend).toContain('未知模型')
    expect(legend).toContain('other')
    expect(legend).toContain('claude-sonnet-4')
    expect(wrapper.find('[data-testid="token-model-composition"]').classes()).toContain('sm:grid-cols-[120px_1fr]')
    const swatch = wrapper.find('[data-testid="token-model-legend"] span.h-2\\.5')
    expect(swatch.exists()).toBe(true)
    wrapper.unmount()
  })

  it('empty models: elevated placeholder, no ECharts host (g2.2)', () => {
    const wrapper = mount(TokenModelComposition, {
      props: { models: [] },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect(pie.find('[data-testid="token-model-pie-empty"]').exists()).toBe(true)
    expect(pie.find('[data-testid="mock-vchart"]').exists()).toBe(false)
    expect(wrapper.html()).not.toMatch(/conic-gradient/i)
    expect(wrapper.find('[data-testid="token-model-legend"]').text()).toMatch(/./)
    wrapper.unmount()
  })
})

describe('TokenModelComposition hides filledTag (g2.1)', () => {
  it('zh-CN legend: swatch+name only, no 含补全; filled=#34D399 unknown=#71717A', () => {
    expect(colorForModel(screenshotLikeModels[0]!, 0)).toBe('#71717A')
    expect(colorForModel(screenshotLikeModels[1]!, 1)).toBe('#34D399')
    expect(colorForModel(screenshotLikeModels[2]!, 2)).toBe('#34D399')

    const wrapper = mount(TokenModelComposition, {
      props: { models: screenshotLikeModels },
      global: { plugins: [i18n()] },
    })
    const legend = wrapper.find('[data-testid="token-model-legend"]')
    const text = legend.text()
    expect(text).toContain('未知模型')
    expect(text).toContain('cursor-grok-4.5-high-fast')
    expect(text).toContain('gpt-5.6-sol-medium')
    expectNoFilledTagCopy(text)
    expect(wrapper.html()).not.toContain('含补全')

    const items = legend.findAll('li')
    expect(items).toHaveLength(3)
    const unknownSwatch = items[0]!.find('span.h-2\\.5')
    const filledSwatch = items[1]!.find('span.h-2\\.5')
    expect(unknownSwatch.attributes('style')).toMatch(/background:\s*#71717A/i)
    expect(filledSwatch.attributes('style')).toMatch(/background:\s*#34D399/i)

    const nameSpans = items.map((li) => li.findAll('span')[1]!)
    expect(nameSpans[0]!.classes()).toContain('text-txt3')
    expect(nameSpans[1]!.classes()).toContain('text-ok')
    expect(nameSpans[2]!.classes()).toContain('text-ok')
    expect(items[0]!.find('[data-testid="unknown-model-badge"]').exists()).toBe(true)

    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills).toEqual(['#71717A', '#34D399', '#34D399'])
    wrapper.unmount()
  })

  it('configured alias: no unknown badge and not #71717A; same-name rows stay distinct (g4.1)', () => {
    const models = [
      { modelKey: '未知/未分桶', name: 'gpt-5', total: 100, unknown: true },
      { modelKey: 'gpt-5', name: 'gpt-5', total: 80 },
    ]
    expect(colorForModel(models[0]!, 0)).toBe('#7B61FF')
    expect(colorForModel(models[0]!, 0)).not.toBe('#71717A')
    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    const legend = wrapper.find('[data-testid="token-model-legend"]')
    expect(legend.text()).toContain('gpt-5')
    expect(legend.find('[data-testid="unknown-model-badge"]').exists()).toBe(false)
    const nameSpan = legend.findAll('li')[0]!.findAll('span')[1]!
    expect(nameSpan.classes()).not.toContain('text-txt3')
    expect(nameSpan.classes()).toContain('text-txt')
    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills[0]).toBe('#7B61FF')
    expect(fills[0]).not.toBe('#71717A')
    expect(legend.findAll('li')).toHaveLength(2)
    wrapper.unmount()
  })

  it('configured alias + filled: sector/legend #34D399 and no badge (g4.1)', () => {
    const models = [
      { modelKey: '未知/未分桶', name: 'Auto', total: 100, unknown: true, filled: true },
      { modelKey: 'cursor-grok', name: 'cursor-grok-4.5-high-fast', total: 80, filled: true },
    ]
    expect(colorForModel(models[0]!, 0)).toBe('#34D399')
    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    const legend = wrapper.find('[data-testid="token-model-legend"]')
    expect(legend.text()).toContain('Auto')
    expect(legend.find('[data-testid="unknown-model-badge"]').exists()).toBe(false)
    const nameSpan = legend.findAll('li')[0]!.findAll('span')[1]!
    expect(nameSpan.classes()).toContain('text-ok')
    expect(nameSpan.classes()).not.toContain('text-txt3')
    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills).toEqual(['#34D399', '#34D399'])
    wrapper.unmount()
  })

  it('en locale legend: no includes filled data; filled/#71717A colors unchanged', () => {
    const wrapper = mount(TokenModelComposition, {
      props: { models: screenshotLikeModels },
      global: { plugins: [i18nEn()] },
    })
    const text = wrapper.find('[data-testid="token-model-legend"]').text()
    expect(text).toContain('cursor-grok-4.5-high-fast')
    expect(text).toContain('gpt-5.6-sol-medium')
    expectNoFilledTagCopy(text)
    expect(wrapper.html()).not.toContain('includes filled data')
    expect(wrapper.html()).not.toContain('含补全')

    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills).toEqual(['#71717A', '#34D399', '#34D399'])
    wrapper.unmount()
  })
})

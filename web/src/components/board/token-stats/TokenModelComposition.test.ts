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
import { setTheme } from '@/lib/shared/theme'

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

function chartOption(vm: unknown) {
  return vm as {
    chartOption: {
      legend?: { orient?: string; right?: number }
      tooltip?: { borderRadius?: number; backgroundColor?: string; appendToBody?: boolean }
      series?: { label?: { show?: boolean; formatter?: string }; center?: string[] }[]
    }
  }
}

function expectPieShell(vm: unknown, theme: 'dark' | 'light') {
  const option = chartOption(vm).chartOption
  expect(option.legend?.orient).toBe('vertical')
  expect(option.legend?.right).toBe(0)
  expect(option.series?.[0]?.label?.show).toBe(true)
  expect(option.series?.[0]?.label?.formatter).toBe('{b} {d}%')
  expect(option.series?.[0]?.center?.[0]).toBe('38%')
  expect(option.tooltip?.borderRadius).toBe(0)
  expect(option.tooltip?.appendToBody).toBe(true)
  expect(JSON.stringify(option.tooltip)).not.toContain('#1a1d23')
  expect(option.tooltip?.backgroundColor).toBe(theme === 'dark' ? '#27272a' : '#ffffff')
}

describe('TokenModelComposition ECharts pie (g1.1/g2.1)', () => {
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
    expect(data[0]!.name).toBe('未知模型')
    expectPieShell(wrapper.vm, 'dark')
    wrapper.unmount()
  })

  it('unknown-only near-full circle: #71717A sector + 100% in option data (g1.2/g2.2)', () => {
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
    expect(data[0]!.name).toBe('未知模型')
    expect(data[0]!.value).toBe(1056240000)
    expect(wrapper.find('[data-testid="token-model-legend"]').exists()).toBe(false)
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
    expect(data.map((d) => d.name)).toEqual(['cursor-grok-4.5-high-fast', '未知模型'])
    expectNoFilledTagCopy(wrapper.html())
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
    const names = chartData(wrapper.vm).map((d) => d.name)
    expect(names).toContain('未知模型')
    expect(names).toContain('other')
    expect(names).toContain('claude-sonnet-4')
    expect(wrapper.find('[data-testid="token-model-composition"]').classes()).toContain('w-full')
    expect(wrapper.find('[data-testid="token-model-composition"]').classes()).not.toContain(
      'sm:grid-cols-[120px_1fr]',
    )
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
  it('zh-CN option data: names only, no 含补全; filled=#34D399 unknown=#71717A', () => {
    expect(colorForModel(screenshotLikeModels[0]!, 0)).toBe('#71717A')
    expect(colorForModel(screenshotLikeModels[1]!, 1)).toBe('#34D399')
    expect(colorForModel(screenshotLikeModels[2]!, 2)).toBe('#34D399')

    const wrapper = mount(TokenModelComposition, {
      props: { models: screenshotLikeModels },
      global: { plugins: [i18n()] },
    })
    expectNoFilledTagCopy(wrapper.html())
    expect(wrapper.html()).not.toContain('含补全')
    const data = chartData(wrapper.vm)
    expect(data.map((d) => d.name)).toEqual([
      '未知模型',
      'cursor-grok-4.5-high-fast',
      'gpt-5.6-sol-medium',
    ])
    expect(data.map((d) => d.color)).toEqual(['#71717A', '#34D399', '#34D399'])
    expectPieShell(wrapper.vm, 'dark')
    wrapper.unmount()
  })

  it('configured alias: no unknown gray; same-name rows stay distinct (g4.1)', () => {
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
    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills[0]).toBe('#7B61FF')
    expect(fills[0]).not.toBe('#71717A')
    expect(chartData(wrapper.vm)).toHaveLength(2)
    expect(chartData(wrapper.vm).every((d) => d.name === 'gpt-5')).toBe(true)
    wrapper.unmount()
  })

  it('configured alias + filled: sector #34D399 (g4.1)', () => {
    const models = [
      { modelKey: '未知/未分桶', name: 'Auto', total: 100, unknown: true, filled: true },
      { modelKey: 'cursor-grok', name: 'cursor-grok-4.5-high-fast', total: 80, filled: true },
    ]
    expect(colorForModel(models[0]!, 0)).toBe('#34D399')
    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    expect(chartData(wrapper.vm)[0]!.name).toBe('Auto')
    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills).toEqual(['#34D399', '#34D399'])
    wrapper.unmount()
  })

  it('en locale option: no includes filled data; filled/#71717A colors unchanged', () => {
    const wrapper = mount(TokenModelComposition, {
      props: { models: screenshotLikeModels },
      global: { plugins: [i18nEn()] },
    })
    expectNoFilledTagCopy(wrapper.html())
    expect(wrapper.html()).not.toContain('includes filled data')
    expect(wrapper.html()).not.toContain('含补全')

    const fills = chartData(wrapper.vm).map((d) => d.color)
    expect(fills).toEqual(['#71717A', '#34D399', '#34D399'])
    wrapper.unmount()
  })

  it('light theme pie tooltip follows shared shell (g2.3)', () => {
    setTheme('light')
    const wrapper = mount(TokenModelComposition, {
      props: { models: screenshotLikeModels },
      global: { plugins: [i18n()] },
    })
    expectPieShell(wrapper.vm, 'light')
    wrapper.unmount()
    setTheme('dark')
  })
})

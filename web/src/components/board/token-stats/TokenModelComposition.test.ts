// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import TokenModelComposition from './TokenModelComposition.vue'
import { colorForModel } from './tokenModelColors'

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

describe('TokenModelComposition SVG solid pie (g1/g2)', () => {
  it('renders svg/path solid pie without conic-gradient or rounded-full (g1.1/g1.2/g2.1)', () => {
    const wrapper = mount(TokenModelComposition, {
      props: {
        models: [{ modelKey: '未知/未分桶', name: '未知模型', total: 1056, unknown: true }],
      },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect(pie.exists()).toBe(true)
    expect(pie.find('svg').exists()).toBe(true)
    const paths = pie.findAll('path')
    expect(paths.length).toBeGreaterThanOrEqual(1)
    // Solid pie: path starts from center (M cx cy L …), not a donut ring
    expect(paths[0]!.attributes('d')).toMatch(/^M\s+55\s+55\s+L/)
    expect(wrapper.html()).not.toMatch(/conic-gradient/i)
    expect(wrapper.html()).not.toMatch(/rounded-full/)
    expect(wrapper.html()).not.toMatch(/inset\s+0\s+0\s+0\s+28px/)
    // No inner hole circle masking the center
    expect(pie.findAll('circle').length).toBe(0)
    wrapper.unmount()
  })

  it('unknown-only near-full circle: solid #71717A path + legend 100% (g1.2/g2.2)', () => {
    const unknown = { modelKey: '未知/未分桶', name: '未知模型', total: 1056240000, unknown: true }
    expect(colorForModel(unknown, 0)).toBe('#71717A')

    const wrapper = mount(TokenModelComposition, {
      props: { models: [unknown] },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    const path = pie.find('path')
    expect(path.exists()).toBe(true)
    expect(path.attributes('fill')).toBe('#71717A')
    // Full circle uses two semicircle arcs (Demo describeSlice)
    expect(path.attributes('d')).toMatch(/A\s+55\s+55\s+0\s+1\s+1/)
    const legend = wrapper.find('[data-testid="token-model-legend"]')
    expect(legend.text()).toContain('未知模型')
    expect(legend.text()).toContain('100%')
    expect(legend.text()).toContain('1.06B')
    wrapper.unmount()
  })

  it('thin wedge + dominant bucket: circular solid sectors (g2.2 attach-like)', () => {
    const models = [
      { modelKey: 'cursor-grok', name: 'cursor-grok-4.5-high-fast', total: 50090000, filled: true },
      { modelKey: '未知/未分桶', name: '未知模型', total: 1059710000, unknown: true },
    ]
    const wrapper = mount(TokenModelComposition, {
      props: { models },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    const paths = pie.findAll('[data-testid="token-model-pie-slice"]')
    expect(paths.length).toBe(2)
    expect(paths.every((p) => p.attributes('d')?.startsWith('M 55 55'))).toBe(true)
    expect(paths[0]!.attributes('fill')).toBe(colorForModel(models[0]!, 0))
    expect(paths[1]!.attributes('fill')).toBe('#71717A')
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

  it('multi-bucket: svg sectors keep unknown #71717A and other #A1A1AA (g1.3/g2.1/g2.2)', () => {
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
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    const paths = pie.findAll('path')
    expect(paths.length).toBe(3)
    const fills = paths.map((p) => p.attributes('fill'))
    expect(fills).toContain('#71717A')
    expect(fills).toContain('#A1A1AA')
    expect(pie.findAll('circle').length).toBe(0)
    const legend = wrapper.find('[data-testid="token-model-legend"]').text()
    expect(legend).toContain('未知模型')
    expect(legend).toContain('other')
    expect(legend).toContain('claude-sonnet-4')
    // Layout: square swatches in legend, left pie + right legend grid
    expect(wrapper.find('[data-testid="token-model-composition"]').classes()).toContain('sm:grid-cols-[120px_1fr]')
    const swatch = wrapper.find('[data-testid="token-model-legend"] span.h-2\\.5')
    expect(swatch.exists()).toBe(true)
    wrapper.unmount()
  })

  it('empty models: elevated circle placeholder, no square conic block (g2.2)', () => {
    const wrapper = mount(TokenModelComposition, {
      props: { models: [] },
      global: { plugins: [i18n()] },
    })
    const pie = wrapper.find('[data-testid="token-model-pie"]')
    expect(pie.find('svg').exists()).toBe(true)
    expect(pie.findAll('path').length).toBe(0)
    expect(pie.find('circle').exists()).toBe(true)
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

    const fills = wrapper.findAll('[data-testid="token-model-pie-slice"]').map((p) => p.attributes('fill'))
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
    const fills = wrapper.findAll('[data-testid="token-model-pie-slice"]').map((p) => p.attributes('fill'))
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
    const fills = wrapper.findAll('[data-testid="token-model-pie-slice"]').map((p) => p.attributes('fill'))
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

    const fills = wrapper.findAll('[data-testid="token-model-pie-slice"]').map((p) => p.attributes('fill'))
    expect(fills).toEqual(['#71717A', '#34D399', '#34D399'])
    wrapper.unmount()
  })
})

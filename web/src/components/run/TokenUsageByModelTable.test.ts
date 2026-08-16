// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenUsageByModelTable from './TokenUsageByModelTable.vue'

function mountTable(props: Record<string, unknown> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(TokenUsageByModelTable, {
    props,
    global: { plugins: [i18n] },
  })
}

const sampleUsage = {
  inputTokens: 907470,
  outputTokens: 270185,
  cacheReadTokens: 24755867,
  cacheWriteTokens: 1816800,
}

const sampleByModel = {
  '未知/未分桶': { ...sampleUsage, source: 'unknown' as const },
  'claude-sonnet-4-5-20250929-thinking-extended-context': {
    inputTokens: 512300,
    outputTokens: 188420,
    cacheReadTokens: 12004551,
    cacheWriteTokens: 902140,
    source: 'bridge' as const,
    filled: true,
  },
}

describe('TokenUsageByModelTable narrow layout', () => {
  it('removes seven-col table and min-w-[520px] / overflow-x-auto defaults', () => {
    const wrapper = mountTable({
      usage: sampleUsage,
      usageByModel: { '未知/未分桶': { ...sampleUsage, source: 'unknown' } },
    })
    const root = wrapper.get('[data-testid="run-token-by-model"]')
    expect(wrapper.find('table').exists()).toBe(false)
    expect(root.html()).not.toContain('min-w-[520px]')
    expect(root.html()).not.toContain('overflow-x-auto')
    expect(root.classes()).toContain('overflow-x-clip')
    // ~480px content width: no table min-width that would force inner horizontal scroll
    expect(root.element.scrollWidth).toBeLessThanOrEqual(root.element.clientWidth + 1)
    wrapper.unmount()
  })

  it('renders model row: name + source + subtotal, composition bar without %, English quad labels', () => {
    const wrapper = mountTable({
      usage: sampleUsage,
      usageByModel: { '未知/未分桶': { ...sampleUsage, source: 'unknown' } },
    })
    const row = wrapper.get('[data-unknown="1"]')
    expect(row.text()).toContain('未知/未分桶')
    expect(row.text()).toMatch(/27\.75M/)
    expect(row.text()).not.toMatch(/%/)
    expect(row.text()).toContain('input')
    expect(row.text()).toContain('output')
    expect(row.text()).toContain('cacheRead')
    expect(row.text()).toContain('cacheWrite')
    // compact display + exact title on subtotal
    const sub = row.find('b')
    expect(sub.text()).toBe('27.75M')
    expect(sub.element.parentElement?.getAttribute('title') || '').toContain('27,750,322')
    wrapper.unmount()
  })

  it('preserves Σ, unknown/bridge anchors and data-* contracts', () => {
    const total = {
      inputTokens: 907470 + 512300,
      outputTokens: 270185 + 188420,
      cacheReadTokens: 24755867 + 12004551,
      cacheWriteTokens: 1816800 + 902140,
    }
    const wrapper = mountTable({
      usage: total,
      usageByModel: sampleByModel,
      unknownModelDisplayName: 'Auto',
    })
    const root = wrapper.get('[data-testid="run-token-by-model"]')
    expect(root.text()).toMatch(/Σ/)
    expect(wrapper.find('[data-unknown="1"]').exists()).toBe(true)
    expect(wrapper.find('[data-filled="1"]').exists()).toBe(true)
    expect(wrapper.find('[data-model="未知/未分桶"]').exists()).toBe(true)
    expect(wrapper.find('[data-filled="1"]').text()).toContain('via ACP_BRIDGE_MODEL')
    // aliased unknown: shows Auto, no badge
    const unk = wrapper.get('[data-unknown="1"]')
    expect(unk.text()).toContain('Auto')
    expect(unk.find('[data-testid="unknown-model-badge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows empty shell when usage is null', () => {
    const wrapper = mountTable({ usage: null, usageByModel: null })
    expect(wrapper.find('[data-testid="run-token-by-model"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('uses compact figures with fmtTokenCount titles on quad cells', () => {
    const wrapper = mountTable({
      usage: sampleUsage,
      usageByModel: { '未知/未分桶': { ...sampleUsage, source: 'unknown' } },
    })
    const titles = wrapper.findAll('[title$="token"]').map((n) => n.attributes('title') || '')
    expect(titles.some((t) => t.includes('907,470'))).toBe(true)
    expect(titles.some((t) => t.includes('24,755,867'))).toBe(true)
    expect(wrapper.text()).toContain('907.5K')
    expect(wrapper.text()).toContain('24.76M')
    wrapper.unmount()
  })

  it('stays readable at ~480px and stacks at ≤320px without min-width overflow', () => {
    const longName = 'claude-sonnet-4-5-20250929-thinking-extended-context'
    const wrapper = mountTable({
      usage: {
        inputTokens: 512300,
        outputTokens: 188420,
        cacheReadTokens: 12004551,
        cacheWriteTokens: 902140,
      },
      usageByModel: {
        [longName]: {
          inputTokens: 512300,
          outputTokens: 188420,
          cacheReadTokens: 12004551,
          cacheWriteTokens: 902140,
          source: 'upstream',
        },
      },
    })
    const root = wrapper.get('[data-testid="run-token-by-model"]').element as HTMLElement
    const row = wrapper.get(`[data-model="${longName}"]`)
    expect(row.find('.truncate').attributes('title')).toBe(longName)
    expect(row.text()).toContain('input')
    expect(row.text()).toContain('output')
    expect(row.text()).toContain('cacheRead')
    expect(row.text()).toContain('cacheWrite')

    for (const width of [480, 320]) {
      Object.defineProperty(root, 'clientWidth', { configurable: true, value: width })
      Object.defineProperty(root, 'scrollWidth', { configurable: true, value: width })
      expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth + 1)
    }
    // auto-fit grid present (inline style minmax ~96px)
    expect(wrapper.html()).toContain('minmax(96px, 1fr)')
    wrapper.unmount()
  })
})

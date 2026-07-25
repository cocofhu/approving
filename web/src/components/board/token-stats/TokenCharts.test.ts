// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import TokenTrendChart from './TokenTrendChart.vue'
import TokenDonutChart from './TokenDonutChart.vue'
import TokenWorkflowRank from './TokenWorkflowRank.vue'

const i18n = () =>
  createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })

describe('Token charts (g2.3/g2.4)', () => {
  it('renders trend svg from buckets including a zero day', () => {
    const wrapper = mount(TokenTrendChart, {
      props: {
        bucketWidth: 'day',
        trend: [
          {
            bucket: '2026-07-24',
            total: 100,
            inputTokens: 40,
            outputTokens: 30,
            cacheReadTokens: 20,
            cacheWriteTokens: 10,
          },
          {
            bucket: '2026-07-25',
            total: 0,
            inputTokens: 0,
            outputTokens: 0,
            cacheReadTokens: 0,
            cacheWriteTokens: 0,
          },
        ],
      },
      global: { plugins: [i18n()] },
    })
    expect(wrapper.find('[data-testid="token-trend-svg"]').exists()).toBe(true)
    expect(wrapper.findAll('circle').length).toBe(2)
    wrapper.unmount()
  })

  it('renders donut ring with legend and center total', () => {
    const wrapper = mount(TokenDonutChart, {
      props: {
        composition: {
          inputTokens: 70,
          outputTokens: 55,
          cacheReadTokens: 35,
          cacheWriteTokens: 20,
          total: 180,
        },
      },
      global: { plugins: [i18n()] },
    })
    expect(wrapper.find('[data-testid="token-donut-svg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="token-donut-legend"]').text()).toContain('input')
    expect(wrapper.text()).toContain('总量')
    wrapper.unmount()
  })

  it('renders Top10 rank bars and styles other distinctly without padding placeholders', () => {
    const wrapper = mount(TokenWorkflowRank, {
      props: {
        workflows: [
          { workflowId: 'a', name: 'approve-main', total: 100 },
          { workflowId: 'b', name: 'doc-review', total: 40 },
          { name: 'other', total: 20, other: true },
        ],
      },
      global: { plugins: [i18n()] },
    })
    const items = wrapper.findAll('li')
    expect(items).toHaveLength(3)
    expect(wrapper.find('.token-rank-other').exists()).toBe(true)
    expect(wrapper.text()).toContain('其他')
    wrapper.unmount()
  })
})

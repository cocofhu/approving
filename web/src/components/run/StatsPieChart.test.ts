// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StatsPieChart, { type PieSlice } from './StatsPieChart.vue'

function mountChart(items: PieSlice[], opts: { emptyLabel?: string; footnote?: string } = {}) {
  return mount(StatsPieChart, {
    props: {
      items,
      centerValue: '2m',
      centerSub: '总耗时',
      emptyLabel: opts.emptyLabel,
      footnote: opts.footnote,
    },
    global: {},
  })
}

describe('StatsPieChart', () => {
  it('renders pie slices and legend', () => {
    const items: PieSlice[] = [
      { key: 'a', label: '调研', durationSec: 40, color: '#6366f1', sharePct: 40 },
      { key: 'b', label: '澄清', durationSec: 60, color: '#22c55e', sharePct: 60 },
    ]
    const wrapper = mountChart(items)
    expect(wrapper.find('[data-testid="stats-pie-svg"]').exists()).toBe(true)
    expect(wrapper.findAll('path').length).toBeGreaterThan(0)
    expect(wrapper.text()).toContain('调研')
    expect(wrapper.text()).toContain('澄清')
    expect(wrapper.text()).toContain('2m')
    wrapper.unmount()
  })

  it('shows empty label when no duration', () => {
    const wrapper = mountChart([], { emptyLabel: '暂无数据' })
    expect(wrapper.text()).toContain('暂无数据')
    expect(wrapper.findAll('path').length).toBe(0)
    wrapper.unmount()
  })

  it('shows footnote when provided', () => {
    const items: PieSlice[] = [
      { key: 'a', label: 'A', durationSec: 10, color: '#000', sharePct: 100 },
    ]
    const wrapper = mountChart(items, { footnote: '相对节点耗时' })
    expect(wrapper.text()).toContain('相对节点耗时')
    wrapper.unmount()
  })
})

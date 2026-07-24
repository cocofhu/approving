// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { StateTraceEntry } from '@/lib/types'
import StateTracePanel from './StateTracePanel.vue'

function mountPanel(trace: StateTraceEntry[] = []) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(StateTracePanel, {
    props: { trace },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('StateTracePanel', () => {
  it('shows empty state', () => {
    const wrapper = mountPanel([])
    expect(wrapper.text()).toMatch(/暂无|没有/)
    wrapper.unmount()
  })

  it('renders trace entries with event labels', () => {
    const trace: StateTraceEntry[] = [
      { event: 'enter', nodeId: 'research', at: '2026-07-18T00:00:00Z', iteration: 1 },
      { event: 'exit', nodeId: 'research', at: '2026-07-18T00:01:00Z', detail: 'completed' },
      { event: 'transition', nodeId: 'gate', at: '2026-07-18T00:02:00Z', to: 'react' },
    ]
    const wrapper = mountPanel(trace)
    expect(wrapper.text()).toContain('research')
    expect(wrapper.text()).toContain('gate')
    expect(wrapper.text()).toContain('react')
    expect(wrapper.text()).toContain('completed')
    wrapper.unmount()
  })

  it('shows iteration badge for re-entry', () => {
    const trace: StateTraceEntry[] = [
      { event: 'enter', nodeId: 'implement', at: '2026-07-18T00:00:00Z', iteration: 2 },
    ]
    const wrapper = mountPanel(trace)
    expect(wrapper.text()).toMatch(/第\s*2\s*次|iteration/i)
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { WFEdge, WFNode } from '@/lib/types'
import OutputSourcesEditor from './OutputSourcesEditor.vue'

const EMPTY_AVAILABLE =
  '连接上游节点后可选。将分支、测试/评审门禁或人工门禁连到本输出节点后，可映射的结构化产物会出现在此列表。'

function mountEditor(opts?: { withUpstream?: boolean; results?: string[] }) {
  const node: WFNode = {
    id: 'output',
    type: 'output',
    label: '输出',
    position: { x: 0, y: 0 },
    config: { results: opts?.results ?? [] },
  }
  const upstream: WFNode = {
    id: 'research',
    type: 'research',
    label: '调研',
    position: { x: -200, y: 0 },
    config: {},
  }
  const edges: WFEdge[] =
    opts?.withUpstream === false
      ? []
      : [{ id: 'e1', source: 'research', target: 'output', kind: 'success' }]
  const allNodes = opts?.withUpstream === false ? [node] : [node, upstream]
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(OutputSourcesEditor, {
    props: { node, allNodes, edges },
    global: { plugins: [i18n], stubs: { Icon: true } },
  })
}

describe('OutputSourcesEditor', () => {
  it('initializes results array and renders editor', () => {
    const wrapper = mountEditor()
    expect(Array.isArray(wrapper.props('node').config.results)).toBe(true)
    expect(wrapper.text().length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('shows Demo empty-available guide when no mappable options', () => {
    // plan_coverage: g2.2 / g3.2 — availableOptions 为空时展示 Demo 长文案
    const wrapper = mountEditor({ withUpstream: false })
    const empty = wrapper.find('[data-testid="output-sources-empty-available"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toBe(EMPTY_AVAILABLE)
    expect(wrapper.text()).toContain('未选择任何来源')
    wrapper.unmount()
  })

  it('hides empty-available guide when options exist', () => {
    // plan_coverage: g2.2 / g3.2 — 非空时不展示空态引导
    const wrapper = mountEditor({ withUpstream: true })
    expect(wrapper.find('[data-testid="output-sources-empty-available"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(EMPTY_AVAILABLE)
    expect(wrapper.text()).toContain('调研')
    wrapper.unmount()
  })
})

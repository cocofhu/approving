// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { WFEdge, WFNode } from '@/lib/types'
import OutputSourcesEditor from './OutputSourcesEditor.vue'

function mountEditor() {
  const node: WFNode = {
    id: 'output',
    type: 'output',
    label: '输出',
    position: { x: 0, y: 0 },
    config: { results: [] },
  }
  const upstream: WFNode = {
    id: 'research',
    type: 'research',
    label: '调研',
    position: { x: -200, y: 0 },
    config: {},
  }
  const edges: WFEdge[] = [{ id: 'e1', source: 'research', target: 'output', kind: 'success' }]
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(OutputSourcesEditor, {
    props: { node, allNodes: [node, upstream], edges },
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
})

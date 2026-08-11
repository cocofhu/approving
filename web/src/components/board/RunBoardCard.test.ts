// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run } from '@/lib/shared/types'
import RunBoardCard from './RunBoardCard.vue'

function sampleRun(): Run {
  return {
    id: 'run-abc123',
    title: '测试运行',
    workflowId: 'wf-1',
    workflowName: 'demo',
    status: 'running',
    progress: 40,
    currentNodeLabel: '调研',
    createdAt: '2026-07-18T00:00:00Z',
    nodes: [],
    edges: [],
    nodeStates: {},
    artifacts: [],
  } as unknown as Run
}

describe('RunBoardCard', () => {
  it('renders run title and emits select on click', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const run = sampleRun()
    const wrapper = mount(RunBoardCard, {
      props: { run },
      global: { plugins: [i18n], stubs: { StatusPill: { template: '<span />' } } },
    })
    expect(wrapper.text()).toContain('测试运行')
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([run])
    wrapper.unmount()
  })
})

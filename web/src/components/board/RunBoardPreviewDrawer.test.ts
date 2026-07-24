// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Run } from '@/lib/types'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

import RunBoardPreviewDrawer from './RunBoardPreviewDrawer.vue'

function sampleRun(): Run {
  return {
    id: 'run-1',
    title: '预览运行',
    workflowId: 'wf-1',
    workflowName: 'demo',
    status: 'completed',
    startedAt: '2026-07-18T00:00:00Z',
    durationSec: 120,
    createdAt: '2026-07-18T00:00:00Z',
    nodes: [],
    edges: [],
    nodeStates: {},
    artifacts: [],
  } as unknown as Run
}

describe('RunBoardPreviewDrawer', () => {
  it('shows run details when open', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const wrapper = mount(RunBoardPreviewDrawer, {
      props: { open: true, run: sampleRun() },
      global: {
        plugins: [i18n],
        stubs: {
          AppDrawer: {
            props: ['open', 'title'],
            template: '<div v-if="open"><slot /><slot name="footer" /></div>',
          },
          AppButton: { template: '<button><slot /></button>' },
          StatusPill: { template: '<span />' },
        },
      },
      attachTo: document.body,
    })
    expect(wrapper.text()).toContain('预览运行')
    wrapper.unmount()
  })
})

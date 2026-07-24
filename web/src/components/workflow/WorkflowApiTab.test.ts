// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Workflow } from '@/lib/types'

const apiMocks = vi.hoisted(() => ({
  listAPIKeys: vi.fn(),
  createAPIKey: vi.fn(),
  revokeAPIKey: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listAPIKeys: apiMocks.listAPIKeys,
      createAPIKey: apiMocks.createAPIKey,
      revokeAPIKey: apiMocks.revokeAPIKey,
    },
  }
})

import WorkflowApiTab from './WorkflowApiTab.vue'

function sampleWorkflow(overrides: Partial<Workflow> = {}): Workflow {
  return {
    id: 'wf-1',
    name: 'demo',
    status: 'published',
    description: '',
    nodes: [],
    edges: [],
    createdAt: '2026-07-18T00:00:00Z',
    updatedAt: '2026-07-18T00:00:00Z',
    ...overrides,
  } as Workflow
}

function mountTab(workflow = sampleWorkflow()) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(WorkflowApiTab, {
    props: { workflow },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button><slot /></button>' },
        AppModal: { template: '<div><slot /></div>' },
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listAPIKeys.mockResolvedValue([])
})

describe('WorkflowApiTab', () => {
  it('shows API base URL for published workflow', async () => {
    const wrapper = mountTab()
    await flushPromises()
    expect(wrapper.text()).toContain('/v1')
    wrapper.unmount()
  })

  it('loads API keys on mount for published workflow', async () => {
    mountTab()
    await flushPromises()
    expect(apiMocks.listAPIKeys).toHaveBeenCalledWith('wf-1')
  })

  it('skips key loading for draft workflow', async () => {
    mountTab(sampleWorkflow({ status: 'draft' }))
    await flushPromises()
    expect(apiMocks.listAPIKeys).not.toHaveBeenCalled()
  })
})

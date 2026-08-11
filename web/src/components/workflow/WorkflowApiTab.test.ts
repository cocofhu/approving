// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import type { Workflow } from '@/lib/shared/types'

const apiMocks = vi.hoisted(() => ({
  listAPIKeys: vi.fn(),
  createAPIKey: vi.fn(),
  revokeAPIKey: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
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

function mountTab(workflow = sampleWorkflow(), locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n =
    locale === 'zh-CN'
      ? createI18n({
          legacy: false,
          locale: 'zh-CN',
          messages: { 'zh-CN': { ...common, ...pages } },
        })
      : createI18n({
          legacy: false,
          locale: 'en',
          messages: { en: { ...enCommon, ...enPages } },
        })
  return mount(WorkflowApiTab, {
    props: { workflow },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button><slot /></button>' },
        AppModal: { template: '<div><slot /><slot name="footer" /></div>' },
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

  it('uses user-facing out-of-scope copy without MVP/ReAct jargon', async () => {
    const wrapper = mountTab()
    await flushPromises()
    expect(wrapper.text()).toContain('暂未提供')
    expect(wrapper.text()).toContain('外部恢复人工审批')
    expect(wrapper.text()).not.toContain('MVP')
    expect(wrapper.text()).not.toContain('ReAct')
    expect(wrapper.text()).not.toContain('resume 门禁')
    wrapper.unmount()
  })

  it('switches out-of-scope section to English via i18n', async () => {
    const wrapper = mountTab(sampleWorkflow(), 'en')
    await flushPromises()
    expect(wrapper.text()).toContain('Not available yet')
    expect(wrapper.text()).toContain('external resume of human approval')
    expect(wrapper.text()).not.toContain('暂未提供')
    expect(wrapper.text()).not.toContain('MVP')
    expect(wrapper.text()).not.toContain('ReAct')
    wrapper.unmount()
  })
})

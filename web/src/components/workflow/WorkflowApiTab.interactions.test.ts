// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Workflow } from '@/lib/shared/types'

const mocks = vi.hoisted(() => ({
  listAPIKeys: vi.fn(), createAPIKey: vi.fn(), revokeAPIKey: vi.fn(),
  success: vi.fn(), error: vi.fn(),
}))
vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return { ...actual, api: { ...actual.api, listAPIKeys: mocks.listAPIKeys, createAPIKey: mocks.createAPIKey, revokeAPIKey: mocks.revokeAPIKey } }
})
vi.mock('@/lib/composables/useToast', () => ({ useToast: () => mocks }))
vi.mock('@/lib/run/useWorkflowAskInputs', () => ({
  useWorkflowAskInputs: () => ({ fields: { value: [
    { key: 'count', type: 'number', required: true, desc: 'Count' },
    { key: 'enabled', type: 'boolean', required: false, desc: '' },
    { key: 'topic', type: 'text', required: false, desc: 'Topic' },
  ] } }),
}))
import WorkflowApiTab from './WorkflowApiTab.vue'

const workflow = { id: 'wf-1', name: 'Demo', status: 'published', nodes: [], edges: [] } as unknown as Workflow
function mountTab(wf = workflow) {
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { ...common, ...pages } } })
  return mount(WorkflowApiTab, {
    props: { workflow: wf },
    global: { plugins: [i18n], stubs: {
      Icon: true,
      AppButton: { template: '<button v-bind="$attrs"><slot /></button>' },
      AppModal: { props: ['open'], emits: ['close'], template: '<div v-if="open" data-testid="key-modal"><button data-testid="modal-x" @click="$emit(\'close\')" /><slot/><slot name="footer"/></div>' },
    } },
  })
}

describe('WorkflowApiTab interactions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listAPIKeys.mockResolvedValue([{ id: 'k1', name: 'CI', key_prefix: 'wk_x', created_at: '2026-01-01T00:00:00Z' }])
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('loads, creates, copies and revokes API keys', async () => {
    mocks.createAPIKey.mockResolvedValue({ key: 'secret-new' })
    mocks.revokeAPIKey.mockResolvedValue(undefined)
    const w = mountTab()
    await flushPromises()
    expect(w.text()).toContain('CI')
    await (w.vm as any).openCreateKey()
    ;(w.vm as any).keyName = 'automation'
    await (w.vm as any).confirmCreateKey()
    await flushPromises()
    expect(mocks.createAPIKey).toHaveBeenCalledWith('wf-1', 'automation')
    expect((w.vm as any).newKeyPlain).toBe('secret-new')
    ;(w.vm as any).copyText('secret-new', 'newkey')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('secret-new')
    expect(mocks.success).toHaveBeenCalled()
    await (w.vm as any).revokeKey('k1')
    expect(mocks.revokeAPIKey).toHaveBeenCalledWith('wf-1', 'k1')
    w.unmount()
  })

  it('covers API failures, guards, examples, and clipboard rejection', async () => {
    mocks.listAPIKeys.mockRejectedValueOnce(new Error('offline'))
    const w = mountTab()
    await flushPromises()
    expect((w.vm as any).keys).toEqual([])
    mocks.createAPIKey.mockRejectedValueOnce(new Error('create failed'))
    await (w.vm as any).confirmCreateKey()
    mocks.revokeAPIKey.mockRejectedValueOnce(new Error('revoke failed'))
    await (w.vm as any).revokeKey('bad')
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    ;(w.vm as any).copyText('x')
    await flushPromises()
    expect(mocks.error).toHaveBeenCalledTimes(3)
    expect((w.vm as any).examples.createCurl).toContain('"count": "0"')
    expect((w.vm as any).examples.createCurl).toContain('"enabled": "true"')
    expect((w.vm as any).tocSections).toHaveLength(11)
    await w.setProps({ workflow: { ...workflow, id: '', status: 'draft' } })
    await (w.vm as any).loadKeys()
    await (w.vm as any).confirmCreateKey()
    await (w.vm as any).revokeKey('x')
    expect(w.text()).toContain('发布')
    w.unmount()
  })

  it('switches every example to Python and copies endpoint text', async () => {
    const w = mountTab()
    await flushPromises()
    for (const key of ['create', 'get', 'artifacts', 'download', 'cancel']) (w.vm as any).codeLang[key] = 'python'
    await w.vm.$nextTick()
    expect(w.text()).toContain('import requests')
    const copyButtons = w.findAll('button').filter((b) => /复制|Copy/.test(b.text()))
    await copyButtons[0]!.trigger('click')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalled()
    w.unmount()
  })
})

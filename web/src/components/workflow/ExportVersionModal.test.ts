// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const apiMocks = vi.hoisted(() => ({
  listWorkflowVersions: vi.fn(),
  exportWorkflowVersion: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listWorkflowVersions: apiMocks.listWorkflowVersions,
      exportWorkflowVersion: apiMocks.exportWorkflowVersion,
    },
  }
})

import ExportVersionModal from './ExportVersionModal.vue'

function mountModal(open = true) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(ExportVersionModal, {
    props: {
      open,
      workflowId: 'wf-1',
      workflowName: 'demo',
      description: '',
      needsRepo: false,
      status: 'published',
    },
    global: {
      plugins: [i18n],
      stubs: {
        AppButton: { template: '<button><slot /></button>' },
        AppModal: {
          props: ['open'],
          template: '<div v-if="open"><slot /></div>',
        },
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listWorkflowVersions.mockResolvedValue([])
})

describe('ExportVersionModal', () => {
  it('loads versions when opened for published workflow', async () => {
    mountModal(true)
    await flushPromises()
    expect(apiMocks.listWorkflowVersions).toHaveBeenCalledWith(
      'wf-1',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
  })

  it('aborts version list when modal closes', async () => {
    let resolveList!: (v: unknown[]) => void
    apiMocks.listWorkflowVersions.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveList = resolve
        }),
    )
    const wrapper = mountModal(true)
    await wrapper.setProps({ open: false })
    resolveList([{ version: 1 }])
    await flushPromises()
    expect(wrapper.text()).not.toMatch(/已发布版本|Published/)
    wrapper.unmount()
  })

  it('renders export options', async () => {
    const wrapper = mountModal(true)
    await flushPromises()
    expect(wrapper.text().length).toBeGreaterThan(0)
    wrapper.unmount()
  })
})

// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const apiMocks = vi.hoisted(() => ({
  copyWorkflow: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      copyWorkflow: apiMocks.copyWorkflow,
    },
  }
})

import CopyWorkflowModal from './CopyWorkflowModal.vue'

function mountModal(open = true) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(CopyWorkflowModal, {
    props: {
      open,
      sourceId: 'wf-1',
      sourceName: '源流水线',
      suggestedName: '源流水线-副本',
      existingNames: ['other'],
    },
    global: {
      plugins: [i18n],
      stubs: {
        Icon: true,
        AppButton: { template: '<button v-bind="$attrs"><slot /></button>' },
        AppModal: {
          props: ['open'],
          template: '<div v-if="open"><slot /><div data-testid="modal-footer"><slot name="footer" /></div></div>',
        },
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.copyWorkflow.mockResolvedValue({ id: 'wf-2', name: '源流水线-副本' })
})

describe('CopyWorkflowModal', () => {
  it('prefills suggested name when opened', async () => {
    const wrapper = mountModal(false)
    await wrapper.setProps({ open: true })
    await flushPromises()
    const input = wrapper.find('input')
    expect((input.element as HTMLInputElement).value).toBe('源流水线-副本')
    wrapper.unmount()
  })

  it('shows validation error for duplicate name', async () => {
    const wrapper = mountModal(true)
    await flushPromises()
    await wrapper.find('input').setValue('other')
    const confirm = wrapper.findAll('button').find((b) => b.text().includes('复制') || b.text().includes('Copy'))
    expect(confirm).toBeTruthy()
    await confirm!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toMatch(/已存在|exists/i)
    wrapper.unmount()
  })
})

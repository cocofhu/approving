// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const importWorkflow = vi.fn()
const listAgents = vi.fn()
const push = vi.fn()

vi.mock('@/lib/api', () => ({
  api: {
    importWorkflow: (...args: unknown[]) => importWorkflow(...args),
    listAgents: (...args: unknown[]) => listAgents(...args),
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

vi.mock('@/lib/workflowIO', () => ({
  skillProfileIssues: () => [{ name: 'missing-agent', reason: 'missing' }],
}))

vi.mock('@/lib/useProjectContext', () => ({
  readStoredProjectId: () => 'p1',
}))

import { useWorkflowImport } from './useWorkflowImport'

describe('useWorkflowImport', () => {
  beforeEach(() => {
    importWorkflow.mockReset()
    listAgents.mockReset()
    push.mockReset()
    importWorkflow.mockResolvedValue({ id: 'w1', name: 'wf', nodes: [] })
    listAgents.mockResolvedValue([{ name: 'a1' }])
  })

  it('imports workflow file and navigates', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    let api: ReturnType<typeof useWorkflowImport> | null = null
    mount(
      defineComponent({
        setup() {
          api = useWorkflowImport({ dirty: () => true })
          return () => h('div')
        },
      }),
      { global: { plugins: [i18n] } },
    )
    api!.triggerImport()
    expect(api!.showDiscardConfirm.value).toBe(true)
    api!.onDiscardCancel()
    api!.onDiscardConfirm()

    mount(
      defineComponent({
        setup() {
          api = useWorkflowImport()
          return () => h('div')
        },
      }),
      { global: { plugins: [i18n] } },
    )
    const input = document.createElement('input')
    api!.fileInput.value = input
    const file = new File(['{}'], 'wf.json', { type: 'application/json' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api!.handleFileChange({ target: input } as unknown as Event)
    expect(importWorkflow).toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith('/workflows/w1/edit')
  })
})

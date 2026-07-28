// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const importAgent = vi.fn()
vi.mock('@/lib/api', () => ({
  api: { importAgent: (...args: unknown[]) => importAgent(...args) },
}))

vi.mock('@/lib/agentIO', () => ({
  peekAgentZipName: vi.fn(async () => ({ name: 'agent-a' })),
  resolveImportName: (name: string) => name,
  suggestRename: (name: string) => `${name}-2`,
  normalizeAgentName: (name: string) => name,
  validateAgentName: () => null,
}))

import { useAgentImport } from './useAgentImport'
import { peekAgentZipName } from './agentIO'

describe('useAgentImport', () => {
  beforeEach(() => {
    importAgent.mockReset()
    importAgent.mockResolvedValue({ name: 'agent-a' })
    vi.mocked(peekAgentZipName).mockResolvedValue({ name: 'agent-a' })
  })

  it('imports zip and handles dirty/conflict flows', async () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': { ...common, ...pages } },
    })
    const onImported = vi.fn()
    let api: ReturnType<typeof useAgentImport> | null = null
    mount(
      defineComponent({
        setup() {
          api = useAgentImport({
            dirty: () => true,
            agentNames: () => ['agent-a'],
            onImported,
          })
          return () => h('div')
        },
      }),
      { global: { plugins: [i18n] } },
    )

    api!.triggerImport()
    expect(api!.showDiscardConfirm.value).toBe(true)
    api!.onDiscardCancel()
    expect(api!.showDiscardConfirm.value).toBe(false)

    api = null
    mount(
      defineComponent({
        setup() {
          api = useAgentImport({
            dirty: () => false,
            agentNames: () => ['agent-a'],
            onImported,
          })
          return () => h('div')
        },
      }),
      { global: { plugins: [i18n] } },
    )
    const input = document.createElement('input')
    api!.fileInput.value = input
    const file = new File(['z'], 'agent-a.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api!.handleFileChange({ target: input } as unknown as Event)
    expect(api!.showConflict.value).toBe(true)
    api!.selectConflict('overwrite')
    await api!.confirmConflict()
    expect(importAgent).toHaveBeenCalled()
  })
})

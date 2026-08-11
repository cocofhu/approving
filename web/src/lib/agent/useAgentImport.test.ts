// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const importAgent = vi.fn()
const importOrgFolder = vi.fn()
const peekZipPackage = vi.fn()

vi.mock('@/lib/api/api', () => ({
  api: {
    importAgent: (...args: unknown[]) => importAgent(...args),
    importOrgFolder: (...args: unknown[]) => importOrgFolder(...args),
  },
}))

vi.mock('@/lib/agent/agentIO', () => ({
  peekZipPackage: (...args: unknown[]) => peekZipPackage(...args),
  peekAgentZipName: vi.fn(async () => ({ name: 'agent-a' })),
  resolveImportName: (name: string) => name,
  suggestRename: (name: string) => `${name}_v2`,
  normalizeAgentName: (name: string) => name,
  validateAgentName: () => '',
}))

import { useAgentImport } from './useAgentImport'

function mountHook(opts: Partial<Parameters<typeof useAgentImport>[0]> = {}) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  let api: ReturnType<typeof useAgentImport> | null = null
  mount(
    defineComponent({
      setup() {
        api = useAgentImport({
          dirty: () => false,
          agentNames: () => ['agent-a'],
          onImported: vi.fn(),
          onFolderImported: vi.fn(),
          ...opts,
        })
        return () => h('div')
      },
    }),
    { global: { plugins: [i18n] } },
  )
  return api!
}

describe('useAgentImport', () => {
  beforeEach(() => {
    importAgent.mockReset()
    importOrgFolder.mockReset()
    peekZipPackage.mockReset()
    importAgent.mockResolvedValue({ name: 'agent-a' })
    importOrgFolder.mockResolvedValue({ org: { revision: 2, groups: [], agents: {} } })
  })

  it('imports zip and handles dirty/conflict flows', async () => {
    peekZipPackage.mockResolvedValue({ kind: 'agent', name: 'agent-a' })
    const onImported = vi.fn()
    let api = mountHook({ dirty: () => true, onImported })

    api.triggerImport()
    await Promise.resolve()
    expect(api.showDiscardConfirm.value).toBe(true)
    api.onDiscardCancel()
    expect(api.showDiscardConfirm.value).toBe(false)

    api = mountHook({ dirty: () => false, onImported })
    const input = document.createElement('input')
    api.fileInput.value = input
    const file = new File(['z'], 'agent-a.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api.handleFileChange({ target: input } as unknown as Event)
    expect(api.showConflict.value).toBe(true)
    api.selectConflict('overwrite')
    await api.confirmConflict()
    expect(importAgent).toHaveBeenCalled()
    expect(importOrgFolder).not.toHaveBeenCalled()
  })

  it('top-bar folder zip goes to org import without targetGroupId', async () => {
    peekZipPackage.mockResolvedValue({ kind: 'org-folder', agentNames: ['bob', 'carol'] })
    const onFolderImported = vi.fn()
    const api = mountHook({
      agentNames: () => ['agent-a'],
      onFolderImported,
    })
    const input = document.createElement('input')
    api.fileInput.value = input
    const file = new File(['z'], 'folder.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api.handleFileChange({ target: input } as unknown as Event)
    expect(api.showBatchConflict.value).toBe(false)
    expect(importOrgFolder).toHaveBeenCalledWith(file, { targetGroupId: undefined, mode: 'rename' })
    expect(onFolderImported).toHaveBeenCalled()
    expect(importAgent).not.toHaveBeenCalled()
  })

  it('folder zip with name conflicts shows one batch dialog', async () => {
    peekZipPackage.mockResolvedValue({ kind: 'org-folder', agentNames: ['agent-a', 'bob'] })
    const api = mountHook({ agentNames: () => ['agent-a', 'other'] })
    const input = document.createElement('input')
    api.fileInput.value = input
    const file = new File(['z'], 'folder.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api.handleFileChange({ target: input } as unknown as Event)
    expect(api.showBatchConflict.value).toBe(true)
    expect(api.batchConflictNames.value).toEqual(['agent-a'])
    expect(importOrgFolder).not.toHaveBeenCalled()

    await api.confirmBatchOverwrite()
    expect(importOrgFolder).toHaveBeenCalledWith(file, { targetGroupId: undefined, mode: 'overwrite' })
  })

  it('group import rejects single-agent zip and does not write', async () => {
    peekZipPackage.mockResolvedValue({ kind: 'agent', name: 'solo' })
    const api = mountHook()
    const input = document.createElement('input')
    api.fileInput.value = input
    api.triggerGroupImport('g_dev')
    const file = new File(['z'], 'solo.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api.handleFileChange({ target: input } as unknown as Event)
    expect(api.showImportError.value).toBe(true)
    expect(api.importError.value).toMatch(/单 Agent ZIP/)
    expect(api.importError.value).toMatch(/文件夹包/)
    expect(api.importError.value).toMatch(/顶栏/)
    expect(api.importError.value).toMatch(/导入/)
    expect(importAgent).not.toHaveBeenCalled()
    expect(importOrgFolder).not.toHaveBeenCalled()
  })

  it('group import of folder zip passes targetGroupId', async () => {
    peekZipPackage.mockResolvedValue({ kind: 'org-folder', agentNames: ['bob'] })
    const api = mountHook({ agentNames: () => [] })
    const input = document.createElement('input')
    api.fileInput.value = input
    api.triggerGroupImport('g_dev')
    const file = new File(['z'], 'folder.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api.handleFileChange({ target: input } as unknown as Event)
    expect(importOrgFolder).toHaveBeenCalledWith(file, { targetGroupId: 'g_dev', mode: 'rename' })
  })

  it('unrecognized zip shows visible error and does not write', async () => {
    peekZipPackage.mockResolvedValue({ kind: 'unknown', error: 'unrecognized' })
    const api = mountHook()
    const input = document.createElement('input')
    api.fileInput.value = input
    const file = new File(['z'], 'mystery.zip', { type: 'application/zip' })
    Object.defineProperty(input, 'files', { value: [file] })
    await api.handleFileChange({ target: input } as unknown as Event)
    expect(api.showImportError.value).toBe(true)
    expect(api.importError.value).toMatch(/无法识别/)
    expect(importAgent).not.toHaveBeenCalled()
    expect(importOrgFolder).not.toHaveBeenCalled()
  })
})

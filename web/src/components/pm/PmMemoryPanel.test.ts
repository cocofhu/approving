// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import { useAuth } from '@/lib/useAuth'
import PmMemoryPanel from './PmMemoryPanel.vue'

const apiMocks = vi.hoisted(() => ({
  listPmMemories: vi.fn(),
  upsertPmMemory: vi.fn(),
  updatePmMemory: vi.fn(),
  deletePmMemory: vi.fn(),
  clearPmMemories: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listPmMemories: apiMocks.listPmMemories,
      upsertPmMemory: apiMocks.upsertPmMemory,
      updatePmMemory: apiMocks.updatePmMemory,
      deletePmMemory: apiMocks.deletePmMemory,
      clearPmMemories: apiMocks.clearPmMemories,
    },
  }
})

vi.mock('@/lib/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(PmMemoryPanel, {
    props: { projectId: 'proj-1' },
    global: { plugins: [i18n] },
  })
}

describe('PmMemoryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listPmMemories.mockResolvedValue({
      items: [
        {
          id: 'm1',
          projectId: 'proj-1',
          agentName: 'agent-a',
          title: '背景',
          content: 'Go',
          source: 'agent',
          updatedBy: 'a',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
      ],
    })
    useAuth().clearUser()
  })

  it('loads memories on mount', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't', isAdmin: true })
    const w = mountPanel()
    await flushPromises()
    expect(apiMocks.listPmMemories).toHaveBeenCalledWith('proj-1')
    expect(w.text()).toContain('背景')
    expect(w.text()).toContain('Go')
  })

  it('hides write/clear controls for non-admin', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't', isAdmin: false })
    const w = mountPanel()
    await flushPromises()
    expect(apiMocks.listPmMemories).toHaveBeenCalledWith('proj-1')
    expect(w.find('textarea').exists()).toBe(false)
    expect(w.find('input').exists()).toBe(false)
    expect(w.text()).toMatch(/只读|readonly|管理员/i)
    const actionBtns = w.findAll('button').map((b) => b.text())
    expect(actionBtns.some((t) => /清空|Clear/i.test(t))).toBe(false)
    expect(actionBtns.some((t) => /编辑|Edit|删除|Delete/i.test(t))).toBe(false)
  })

  it('shows admin form and clear when items exist', async () => {
    useAuth().setUser({ username: 'admin', expiresAt: 't', isAdmin: true })
    const w = mountPanel()
    await flushPromises()
    expect(w.find('textarea').exists()).toBe(true)
    const actionBtns = w.findAll('button').map((b) => b.text())
    expect(actionBtns.some((t) => /清空|Clear/i.test(t))).toBe(true)
  })

  it('admin can upsert and delete memories', async () => {
    useAuth().setUser({ username: 'admin', expiresAt: 't', isAdmin: true })
    apiMocks.upsertPmMemory.mockResolvedValue({ id: 'm2', title: '新记' })
    apiMocks.deletePmMemory.mockResolvedValue({ status: 'deleted' })
    apiMocks.listPmMemories
      .mockResolvedValueOnce({
        items: [
          {
            id: 'm1',
            projectId: 'proj-1',
            agentName: 'agent-a',
            title: '背景',
            content: 'Go',
            source: 'agent',
            updatedBy: 'a',
            createdAt: '2026-01-01T00:00:00Z',
            updatedAt: '2026-01-01T00:00:00Z',
          },
        ],
      })
      .mockResolvedValue({ items: [] })

    const w = mountPanel()
    await flushPromises()

    await w.find('input').setValue('新记')
    await w.find('textarea').setValue('内容')
    const addBtn = w.findAll('button').find((b) => /添加|Add|保存|Save/i.test(b.text()))
    expect(addBtn).toBeTruthy()
    await addBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.upsertPmMemory).toHaveBeenCalledWith('proj-1', {
      title: '新记',
      content: '内容',
    })

    // remount with an item to delete
    apiMocks.listPmMemories.mockResolvedValue({
      items: [
        {
          id: 'm1',
          projectId: 'proj-1',
          agentName: 'agent-a',
          title: '背景',
          content: 'Go',
          source: 'agent',
          updatedBy: 'a',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
      ],
    })
    const w2 = mountPanel()
    await flushPromises()
    const delBtn = w2.findAll('button').find((b) => /删除|Delete/i.test(b.text()))
    expect(delBtn).toBeTruthy()
    await delBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.deletePmMemory).toHaveBeenCalledWith('proj-1', 'm1')
  })
})

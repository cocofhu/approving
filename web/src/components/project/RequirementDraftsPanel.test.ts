// @vitest-environment happy-dom
import { defineComponent, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'

const apiMocks = vi.hoisted(() => ({
  listRequirementDrafts: vi.fn(),
  createRequirementDraft: vi.fn(),
  updateRequirementDraft: vi.fn(),
  patchRequirementDraftStatus: vi.fn(),
  deleteRequirementDraft: vi.fn(),
}))

vi.mock('@/lib/api/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/api')>('@/lib/api/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      listRequirementDrafts: apiMocks.listRequirementDrafts,
      createRequirementDraft: apiMocks.createRequirementDraft,
      updateRequirementDraft: apiMocks.updateRequirementDraft,
      patchRequirementDraftStatus: apiMocks.patchRequirementDraftStatus,
      deleteRequirementDraft: apiMocks.deleteRequirementDraft,
    },
  }
})

const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  show: vi.fn(),
}))

vi.mock('@/lib/composables/useToast', () => ({
  useToast: () => toastMocks,
}))

import RequirementDraftsPanel from './RequirementDraftsPanel.vue'

const draftOpen = {
  id: 'rd-1',
  projectId: 'proj-a',
  title: '支付失败重试',
  bodyMarkdown: '## 要点',
  status: 'open' as const,
  createdAt: '2026-08-08T10:12:00Z',
  updatedAt: '2026-08-10T14:22:00Z',
}

const draftOpen2 = {
  ...draftOpen,
  id: 'rd-2',
  title: '另一条草稿',
  bodyMarkdown: '旧正文',
}

function mockMatchMedia(isNarrow: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: isNarrow && String(query).includes('max-width: 767px'),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    }),
  })
}

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(RequirementDraftsPanel, {
    props: { projectId: 'proj-a' },
    attachTo: document.body,
    global: {
      plugins: [i18n],
      stubs: {
        AppButton: defineComponent({
          props: ['disabled'],
          emits: ['click'],
          setup(_, { slots, emit, attrs }) {
            return () =>
              h(
                'button',
                {
                  type: 'button',
                  disabled: _.disabled as boolean | undefined,
                  'data-testid': attrs['data-testid'],
                  onClick: () => emit('click'),
                },
                slots.default?.(),
              )
          },
        }),
        AppModal: defineComponent({
          props: ['open', 'title'],
          emits: ['close'],
          setup(p, { slots }) {
            return () =>
              p.open
                ? h('div', { 'data-testid': 'app-modal' }, [
                    h('div', p.title),
                    slots.default?.(),
                    slots.footer?.(),
                  ])
                : null
          },
        }),
      },
    },
  })
}

describe('RequirementDraftsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMatchMedia(false)
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      cb(0)
      return 1
    })
    vi.stubGlobal('cancelAnimationFrame', () => {})
    apiMocks.listRequirementDrafts.mockResolvedValue({ items: [draftOpen] })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads open drafts by default and shows master-detail', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(apiMocks.listRequirementDrafts).toHaveBeenCalledWith('proj-a', {
      status: 'open',
      q: undefined,
    })
    expect(w.find('[data-testid="requirement-drafts-panel"]').exists()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-empty-detail"]').text()).toContain('未选中草稿')
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    expect(w.get('[data-testid="requirement-drafts-title"]').element).toHaveProperty(
      'value',
      '支付失败重试',
    )
    expect(w.find('[data-testid="requirement-drafts-markdown-split"]').exists()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-status-pill"]').text()).toContain('未完成')
    expect(w.find('[data-testid="requirement-drafts-item-active-bar"]').exists()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-toggle-status"]').text()).toContain('标记为已完成')
    w.unmount()
  })

  it('creates unnamed draft then selects it', async () => {
    const created = {
      ...draftOpen,
      id: 'rd-new',
      title: '未命名需求',
      bodyMarkdown: '',
    }
    apiMocks.createRequirementDraft.mockResolvedValue(created)
    apiMocks.listRequirementDrafts
      .mockResolvedValueOnce({ items: [draftOpen] })
      .mockResolvedValueOnce({ items: [created, draftOpen] })
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-new"]').trigger('click')
    await flushPromises()
    expect(apiMocks.createRequirementDraft).toHaveBeenCalledWith('proj-a')
    expect(w.get('[data-testid="requirement-drafts-title"]').element).toHaveProperty(
      'value',
      '未命名需求',
    )
    w.unmount()
  })

  it('rejects empty title on save', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    const title = w.get('[data-testid="requirement-drafts-title"]')
    await title.setValue('   ')
    await w.get('[data-testid="requirement-drafts-save"]').trigger('click')
    await nextTick()
    expect(apiMocks.updateRequirementDraft).not.toHaveBeenCalled()
    expect(w.get('[data-testid="requirement-drafts-title-error"]').text()).toContain('不能为空')
    w.unmount()
  })

  it('inserts Demo wrap syntax from the cross-pane toolbar', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    expect(w.get('[data-testid="requirement-drafts-toolbar"]').text()).toContain('H1')
    expect(w.get('[data-testid="requirement-drafts-toolbar"]').text()).toContain('· 列')
    expect(w.get('[data-testid="requirement-drafts-toolbar"]').text()).toContain('查找')
    expect(w.get('[data-testid="requirement-drafts-toolbar"]').text()).toContain('折叠预览')
    await w.get('[data-testid="requirement-drafts-body"]').setValue('')
    await w.get('[data-testid="requirement-drafts-tb-bold"]').trigger('click')
    await nextTick()
    expect((w.get('[data-testid="requirement-drafts-body"]').element as HTMLTextAreaElement).value).toBe(
      '**粗体**',
    )
    expect(w.find('[data-testid="requirement-drafts-gutter"]').text()).toContain('1')
    expect(w.find('[data-testid="requirement-drafts-highlight"]').exists()).toBe(true)
    w.unmount()
  })

  it('keeps the buffer when canceling a dirty draft switch', async () => {
    apiMocks.listRequirementDrafts.mockResolvedValue({ items: [draftOpen, draftOpen2] })
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    await w.get('[data-testid="requirement-drafts-title"]').setValue('未保存标题')
    expect(w.get('[data-testid="requirement-drafts-dirty-chip"]').text()).toContain('未保存')
    await w.get('[data-testid="requirement-drafts-item-rd-2"]').trigger('click')
    await nextTick()
    expect(w.text()).toContain('丢弃未保存的修改？')
    await w.get('[data-testid="requirement-drafts-leave-cancel"]').trigger('click')
    await nextTick()
    expect(w.get('[data-testid="requirement-drafts-title"]').element).toHaveProperty('value', '未保存标题')
    expect(w.find('[data-testid="requirement-drafts-dirty-chip"]').exists()).toBe(true)
    w.unmount()
  })

  it('asks before create when dirty and keeps buffer on cancel', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    await w.get('[data-testid="requirement-drafts-title"]').setValue('本地标题')
    await w.get('[data-testid="requirement-drafts-new"]').trigger('click')
    await nextTick()
    expect(apiMocks.createRequirementDraft).not.toHaveBeenCalled()
    expect(w.text()).toContain('丢弃未保存的修改？')
    await w.get('[data-testid="requirement-drafts-leave-cancel"]').trigger('click')
    await nextTick()
    expect(w.get('[data-testid="requirement-drafts-title"]').element).toHaveProperty('value', '本地标题')
    w.unmount()
  })

  it('keeps dirty title after mark-done refresh (keepSelection)', async () => {
    const patched = {
      ...draftOpen,
      status: 'done' as const,
      updatedAt: '2026-08-11T16:00:00Z',
    }
    apiMocks.patchRequirementDraftStatus.mockResolvedValue(patched)
    // 真实 open 筛选：PATCH 后列表不再包含已完成项（g5.2 回归：不得因此卸掉编辑区）
    apiMocks.listRequirementDrafts
      .mockResolvedValueOnce({ items: [draftOpen, draftOpen2] })
      .mockResolvedValueOnce({ items: [draftOpen2] })
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    await w.get('[data-testid="requirement-drafts-title"]').setValue('仍未保存')
    expect(w.get('[data-testid="requirement-drafts-dirty-chip"]').text()).toContain('未保存')
    await w.get('[data-testid="requirement-drafts-toggle-status"]').trigger('click')
    await flushPromises()
    expect(apiMocks.patchRequirementDraftStatus).toHaveBeenCalledWith('proj-a', 'rd-1', 'done')
    expect(w.find('[data-testid="requirement-drafts-empty-detail"]').exists()).toBe(false)
    expect(w.get('[data-testid="requirement-drafts-title"]').element).toHaveProperty('value', '仍未保存')
    expect(w.find('[data-testid="requirement-drafts-dirty-chip"]').exists()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-status-pill"]').text()).toContain('已完成')
    expect(w.find('[data-testid="requirement-drafts-item-rd-1"]').exists()).toBe(false)
    expect(w.find('[data-testid="requirement-drafts-item-rd-2"]').exists()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-toggle-status"]').text()).toContain('标记为未完成')
    w.unmount()
  })

  it('does not stack source and preview in a narrow viewport', async () => {
    mockMatchMedia(true)
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    expect(w.find('[data-testid="requirement-drafts-mobile-switch"]').exists()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-markdown-split"]').classes()).toContain(
      'rd-split-narrow',
    )
    expect(w.get('[data-testid="requirement-drafts-source-pane"]').isVisible()).toBe(true)
    expect(w.get('[data-testid="requirement-drafts-preview-pane"]').isVisible()).toBe(false)
    expect(w.find('[data-testid="requirement-drafts-sash"]').isVisible()).toBe(false)
    await w.get('[data-testid="requirement-drafts-mobile-prev"]').trigger('click')
    await nextTick()
    expect(w.get('[data-testid="requirement-drafts-source-pane"]').isVisible()).toBe(false)
    expect(w.get('[data-testid="requirement-drafts-preview-pane"]').isVisible()).toBe(true)
    w.unmount()
  })

  it('saves via Ctrl/Cmd+S on the editor pane', async () => {
    apiMocks.updateRequirementDraft.mockResolvedValue({
      ...draftOpen,
      title: '快捷键保存',
      bodyMarkdown: '## 要点',
    })
    apiMocks.listRequirementDrafts
      .mockResolvedValueOnce({ items: [draftOpen] })
      .mockResolvedValueOnce({
        items: [{ ...draftOpen, title: '快捷键保存' }],
      })
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    await w.get('[data-testid="requirement-drafts-title"]').setValue('快捷键保存')
    await w.get('[data-testid="requirement-drafts-detail"]').trigger('keydown', {
      key: 's',
      ctrlKey: true,
    })
    await flushPromises()
    expect(apiMocks.updateRequirementDraft).toHaveBeenCalledWith('proj-a', 'rd-1', {
      title: '快捷键保存',
      bodyMarkdown: '## 要点',
    })
    expect(toastMocks.success).toHaveBeenCalledWith('已保存')
    expect(w.find('[data-testid="requirement-drafts-dirty-chip"]').exists()).toBe(false)
    w.unmount()
  })

  it('opens the in-pane find bar from the toolbar', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.get('[data-testid="requirement-drafts-item-rd-1"]').trigger('click')
    await nextTick()
    expect(w.find('[data-testid="requirement-drafts-findbar"]').isVisible()).toBe(false)
    await w.get('[data-testid="requirement-drafts-tb-find"]').trigger('click')
    await nextTick()
    expect(w.find('[data-testid="requirement-drafts-findbar"]').isVisible()).toBe(true)
    w.unmount()
  })
})

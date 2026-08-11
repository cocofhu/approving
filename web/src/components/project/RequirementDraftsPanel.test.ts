// @vitest-environment happy-dom
import { defineComponent, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

function mountPanel() {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(RequirementDraftsPanel, {
    props: { projectId: 'proj-a' },
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
                ? h('div', { 'data-testid': 'delete-modal' }, [
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
    apiMocks.listRequirementDrafts.mockResolvedValue({ items: [draftOpen] })
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
  })
})

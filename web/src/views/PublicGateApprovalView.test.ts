// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import shell from '@/locales/zh-CN/shell.json'
import commonEn from '@/locales/en/common.json'
import pagesEn from '@/locales/en/pages.json'
import shellEn from '@/locales/en/shell.json'
import { setTheme } from '@/lib/theme'

const mocks = vi.hoisted(() => ({
  preview: vi.fn(),
  decide: vi.fn(),
  reply: vi.fn(),
  cancel: vi.fn(),
}))

vi.mock('@/lib/gateShareLink', async () => {
  const actual = await vi.importActual<typeof import('@/lib/gateShareLink')>('@/lib/gateShareLink')
  return {
    ...actual,
    publicGateApi: {
      preview: mocks.preview,
      decide: mocks.decide,
      reply: mocks.reply,
      cancel: mocks.cancel,
    },
  }
})

vi.mock('@/lib/locale', async () => {
  const actual = await vi.importActual<typeof import('@/lib/locale')>('@/lib/locale')
  return {
    ...actual,
    applyPublicLocale: vi.fn().mockResolvedValue(undefined),
  }
})

import PublicGateApprovalView from './PublicGateApprovalView.vue'

function mountView(locale: 'zh-CN' | 'en' = 'zh-CN') {
  const i18n = createI18n({
    legacy: false,
    locale,
    messages: {
      'zh-CN': { ...common, ...pages, ...shell },
      en: { ...commonEn, ...pagesEn, ...shellEn },
    },
  })
  return mount(PublicGateApprovalView, { global: { plugins: [i18n] } })
}

beforeEach(() => {
  mocks.preview.mockReset()
  mocks.decide.mockReset()
  mocks.reply.mockReset()
  mocks.cancel.mockReset()
  window.location.hash = ''
  localStorage.clear()
  setTheme('dark')
})

afterEach(() => {
  document.documentElement.classList.remove('light')
})

describe('PublicGateApprovalView workbench', () => {
  it('renders dark three-pane workbench for human_gate without purple chrome', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'human_gate',
      title: '审阅视觉稿',
      remainingSec: 3600,
      nonce: 'n1',
      reactSessionAlive: true,
      productKind: 'visual',
      productName: 'page.html',
      actions: { approve: 'approve', reject: 'revise', confirm: 'approve', reply: 'reply', cancel: 'cancel' },
      visualHtml: '<p>ok</p>',
      turns: [{ role: 'agent', text: '请审阅 page.html', at: '2026-08-01T00:00:00Z' }],
      upstream: { name: 'clarified_requirement.json', title: '澄清需求', summary: '可对照审阅当前主产物' },
    })
    const w = mountView()
    await flushPromises()
    expect(document.documentElement.classList.contains('light')).toBe(false)
    expect(w.find('[data-testid="public-gate-chrome"]').classes().join(' ')).not.toMatch(/bg-accent/)
    expect(w.get('[data-testid="public-gate-badge"]').text()).toBe('外部一次决策')
    expect(w.get('[data-testid="review-shell"]').exists()).toBe(true)
    expect(w.get('[data-testid="public-gate-product-label"]').text()).toContain('视觉网页产物')
    expect(w.get('[data-testid="public-gate-product-name"]').text()).toBe('page.html')
    expect(w.get('[data-testid="public-gate-footer"]').text()).toContain('上游上下文')
    expect(w.get('[data-testid="public-gate-upstream-enlarge"]').text()).toContain('放大上游上下文')
    expect(w.get('[data-testid="public-gate-confirm"]').text()).toBe('确认并流转')
    expect(w.get('[data-testid="public-gate-reject"]').text()).toBe('驳回')
    expect(w.get('[data-testid="public-gate-confirm-hint"]').text()).toContain('不触发 Agent')
    expect(w.get('[data-testid="public-gate-sidebar"]').text()).toContain('Agent交互')
    expect(w.find('[data-testid="clarify-confirm-flow"]').exists()).toBe(false)
    expect(w.find('[data-testid="clarify-input"]').exists()).toBe(true)
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/请确认本次交付|这是交付预览|不是审批工作台|Approving|打开运行详情|run-/)
    expect(w.find('[data-testid="html-preview-inspect-toggle"]').exists()).toBe(true)
  })

  it('review hot session has ReAct + footer confirm and no reject', async () => {
    window.location.hash = `#t=${'ee'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      remainingSec: 3600,
      nonce: 'n3',
      reactSessionAlive: true,
      productKind: 'structured',
      productName: 'research.json',
      actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
      structured: { name: 'research.json', title: '调研摘要', doc: { title: '调研摘要' } },
      turns: [
        { role: 'agent', text: '请复审', at: '2026-08-01T00:00:00Z' },
        { role: 'human', text: '改摘要', at: '2026-08-01T00:01:00Z' },
      ],
      upstream: { title: '澄清', summary: '已有澄清需求文档' },
    })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-badge"]').text()).toBe('外部复审')
    expect(w.get('[data-testid="public-gate-sidebar"]').text()).toContain('共 2 条')
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-reject"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-name"]').exists()).toBe(false)
    expect(w.find('[data-testid="clarify-confirm-flow"]').exists()).toBe(false)
    expect(w.get('[data-testid="public-gate-footer"]').text()).toContain('放大上游上下文')

    mocks.decide.mockResolvedValue({ status: 'confirmed', action: 'confirm' })
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).toHaveBeenCalledWith(expect.objectContaining({ action: 'confirm' }))
    expect(w.get('[data-testid="public-gate-done"]').text()).toContain('已确认')
  })

  it('cold review keeps three panes, hides send/inspect, footer still confirms', async () => {
    window.location.hash = `#t=${'cc'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      nonce: 'n-cold',
      reactSessionAlive: false,
      productKind: 'visual',
      productName: 'page.html',
      actions: { confirm: 'confirm' },
      visualHtml: '<p>ok</p>',
      turns: [{ role: 'agent', text: '历史回合', at: '2026-08-01T00:00:00Z' }],
    })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="review-shell"]').exists()).toBe(true)
    expect(w.get('[data-testid="public-gate-session-ended"]').text()).toContain('会话已结束')
    expect(w.get('[data-testid="public-gate-cold-hint"]').text()).toContain('仅可确认并流转')
    expect(w.find('[data-testid="clarify-input"]').exists()).toBe(false)
    expect(w.find('[data-testid="html-preview-inspect-toggle"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
    expect(w.get('[data-testid="public-gate-sidebar"]').text()).toContain('历史回合')
  })

  it('human_gate requires name and comment before confirm or reject', async () => {
    window.location.hash = `#t=${'ff'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'human_gate',
      nonce: 'n4',
      reactSessionAlive: false,
      actions: { approve: 'approve', reject: 'revise', confirm: 'approve' },
    })
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).not.toHaveBeenCalled()
    expect(w.get('[data-testid="public-gate-error"]').text()).toContain('姓名与意见')

    await w.get('[data-testid="public-gate-name"]').setValue('Jordan')
    await w.get('[data-testid="public-gate-comment"]').setValue('可以流转')
    mocks.decide.mockResolvedValue({ status: 'approved' })
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.decide).toHaveBeenCalledWith(expect.objectContaining({
      action: 'approve',
      name: 'Jordan',
      comment: '可以流转',
    }))
    expect(w.get('[data-testid="public-gate-done"]').text()).toContain('已确认')
  })

  it('reply uses token adapter and does not call decide', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      nonce: 'n-reply',
      reactSessionAlive: true,
      actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
      turns: [],
    })
    mocks.reply.mockResolvedValue({ status: 'accepted' })
    mocks.preview.mockResolvedValueOnce({
      status: 'active',
      kind: 'review',
      nonce: 'n-reply',
      reactSessionAlive: true,
      actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
      turns: [],
    }).mockResolvedValue({
      status: 'active',
      kind: 'review',
      nonce: 'n-reply',
      reactSessionAlive: true,
      actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
      turns: [
        { role: 'human', text: '改标题', at: '2026-08-01T00:02:00Z' },
      ],
    })
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="clarify-input"]').setValue('改标题')
    await w.get('[data-testid="clarify-send-icon"]').trigger('click')
    await flushPromises()
    expect(mocks.reply).toHaveBeenCalledWith(expect.objectContaining({ token: 'aa'.repeat(32), text: '改标题' }))
    expect(mocks.decide).not.toHaveBeenCalled()
    expect(w.find('[data-testid="public-gate-done"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
  })

  it('busy confirm stays on workbench and keeps the link', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      nonce: 'n-busy',
      reactSessionAlive: true,
      sessionBusy: false,
      actions: { confirm: 'confirm', reply: 'reply' },
    })
    const w = mountView()
    await flushPromises()
    mocks.decide.mockResolvedValueOnce({
      status: 'busy',
      error: 'review_busy',
      message: '复审进行中，请稍后再试',
    })
    await w.get('[data-testid="public-gate-confirm"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="public-gate-error"]').text()).toContain('复审进行中')
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-done"]').exists()).toBe(false)
  })

  it('silent preview without new turns does not drop optimistic ReAct queue', async () => {
    window.location.hash = `#t=${'aa'.repeat(32)}`
    const basePreview = {
      status: 'active',
      kind: 'review',
      nonce: 'n-race',
      reactSessionAlive: true,
      sessionBusy: false,
      waiting: 0,
      actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
      turns: [{ role: 'agent', text: '请复审', at: '2026-08-01T00:00:00Z' }],
    }
    mocks.preview.mockResolvedValue({ ...basePreview })
    let resolveReply: ((value: unknown) => void) | undefined
    mocks.reply.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveReply = resolve
        }),
    )
    const w = mountView()
    await flushPromises()
    await w.get('[data-testid="clarify-input"]').setValue('改标题')
    await w.get('[data-testid="clarify-send-icon"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="clarify-review-queue"]').exists()).toBe(true)
    expect(w.get('[data-testid="clarify-review-queue"]').text()).toContain('改标题')

    mocks.preview.mockResolvedValue({
      ...basePreview,
      sessionBusy: false,
      waiting: 0,
      turns: [{ role: 'agent', text: '请复审', at: '2026-08-01T00:00:00Z' }],
    })
    await (w.vm as unknown as { loadPreview: (opts?: { silent?: boolean }) => Promise<void> }).loadPreview({
      silent: true,
    })
    await flushPromises()
    expect(w.find('[data-testid="clarify-review-queue"]').exists()).toBe(true)
    expect(w.get('[data-testid="clarify-review-queue"]').text()).toContain('改标题')
    expect(w.find('[data-testid="public-gate-done"]').exists()).toBe(false)
    expect(mocks.decide).not.toHaveBeenCalled()

    resolveReply?.({ status: 'accepted' })
    await flushPromises()
    expect(w.find('[data-testid="clarify-review-queue"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(true)
  })

  it('sessionBusy preview restores thinking placeholder and cancel', async () => {
    window.location.hash = `#t=${'dd'.repeat(32)}`
    mocks.preview.mockResolvedValue({
      status: 'active',
      kind: 'review',
      nonce: 'n-busy-resume',
      reactSessionAlive: true,
      sessionBusy: true,
      waiting: 0,
      activeItem: { text: '改标题' },
      actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
      turns: [{ role: 'agent', text: '请复审', at: '2026-08-01T00:00:00Z' }],
    })
    const w = mountView()
    await flushPromises()
    await flushPromises()
    expect(w.find('[data-testid="clarify-busy-placeholder"]').exists()).toBe(true)
    expect(w.find('[data-testid="clarify-review-cancel"]').exists()).toBe(true)
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(false)
    expect(w.find('[data-testid="public-gate-done"]').exists()).toBe(false)
  })

  it('unavailable states keep dark chrome without confirm/send', async () => {
    window.location.hash = `#t=${'bb'.repeat(32)}`
    mocks.preview.mockResolvedValue({ status: 'expired', kind: 'human_gate' })
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="public-gate-badge"]').text()).toBe('外部一次决策')
    expect(w.get('[data-testid="public-gate-invalid"]').text()).toContain('已过期')
    expect(w.find('[data-testid="public-gate-confirm"]').exists()).toBe(false)
    expect(w.find('[data-testid="clarify-input"]').exists()).toBe(false)
    expect(w.get('[data-testid="public-gate-root"]').text()).not.toMatch(/请确认本次交付/)
  })
})

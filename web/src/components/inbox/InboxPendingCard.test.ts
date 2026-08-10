// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import InboxPendingCard from './InboxPendingCard.vue'
import type { ClarifyInboxItem, GateInboxItem } from '@/lib/types'

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { ...common, ...pages } },
})

function gate(over: Partial<GateInboxItem> = {}): GateInboxItem {
  return {
    type: 'gate',
    nodeType: 'human_gate',
    runId: 'run-1',
    nodeId: 'hg1',
    workflowName: 'wf',
    title: '审阅视觉稿',
    bodyMd: '请审阅',
    actions: [{ id: 'approve', label: '批准' }],
    requestedAt: '2026-08-01T00:00:00Z',
    shareLink: { state: 'none', canCreate: true },
    ...over,
  }
}

function clarify(over: Partial<ClarifyInboxItem> = {}): ClarifyInboxItem {
  return {
    type: 'clarify',
    runId: 'run-2',
    nodeId: 'react1',
    workflowName: 'wf',
    label: '澄清需求',
    done: false,
    requestedAt: '2026-08-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
    ...over,
  }
}

describe('InboxPendingCard share entry', () => {
  it('shows copy button and status for human_gate and kind=review only', async () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate() },
      global: { plugins: [i18n] },
    })
    expect(w.get('[data-testid="gate-share-copy-btn"]').text()).toContain('复制临时链接')
    expect(w.get('[data-testid="gate-share-status"]').text()).toContain('尚未创建')
    await w.get('[data-testid="gate-share-copy-btn"]').trigger('click')
    expect(w.emitted('open-share')).toHaveLength(1)

    const review = mount(InboxPendingCard, {
      props: {
        item: clarify({
          kind: 'review',
          nodeId: 'research1',
          label: '调研复审',
          shareLink: { state: 'none', canCreate: true },
        }),
      },
      global: { plugins: [i18n] },
    })
    expect(review.get('[data-testid="gate-share-copy-btn"]').text()).toContain('复制临时链接')
    expect(review.get('[data-testid="gate-share-status"]').text()).toContain('尚未创建')

    const c = mount(InboxPendingCard, {
      props: { item: clarify() },
      global: { plugins: [i18n] },
    })
    expect(c.find('[data-testid="gate-share-copy-btn"]').exists()).toBe(false)
    expect(c.find('[data-testid="gate-share-status"]').exists()).toBe(false)

    const preview = mount(InboxPendingCard, {
      props: { item: clarify({ kind: 'app_preview', label: '应用预览' }) },
      global: { plugins: [i18n] },
    })
    expect(preview.find('[data-testid="gate-share-copy-btn"]').exists()).toBe(false)

    const ps = mount(InboxPendingCard, {
      props: { item: gate({ nodeId: 'ps1', nodeType: 'proposal_select', title: '选择方案' }) },
      global: { plugins: [i18n] },
    })
    expect(ps.find('[data-testid="gate-share-copy-btn"]').exists()).toBe(false)
    expect(ps.find('[data-testid="gate-share-status"]').exists()).toBe(false)
  })

  it('copy button uses chip-sized classes at all breakpoints and is not taller than the status chip', () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate() },
      global: { plugins: [i18n] },
    })
    const btn = w.get('[data-testid="gate-share-copy-btn"]')
    const chip = w.get('[data-testid="gate-share-status"]')
    const row = w.get('[data-testid="gate-share-row"]')
    const cls = btn.classes().join(' ')
    expect(cls).toContain('min-h-6')
    expect(cls).toContain('text-[10px]')
    expect(cls).toContain('px-1.5')
    expect(cls).toContain('py-0.5')
    expect(cls).toContain('gap-1')
    expect(cls).toContain('border-accent/40')
    expect(cls).toContain('bg-accent/10')
    expect(cls).toContain('text-accent-2')
    expect(cls).not.toContain('min-h-11')
    expect(cls).not.toContain('min-w-[44px]')
    expect(cls).not.toContain('text-xs')
    expect(cls).not.toContain('md:min-h-6')
    expect(cls).not.toContain('md:text-[10px]')
    expect(btn.text()).toContain('复制临时链接')
    expect(row.classes()).toEqual(expect.arrayContaining(['flex-wrap', 'items-center', 'gap-2']))
    const btnH = (btn.element as HTMLElement).getBoundingClientRect().height
    const chipH = (chip.element as HTMLElement).getBoundingClientRect().height
    if (btnH > 0 && chipH > 0) {
      expect(btnH).toBeLessThanOrEqual(chipH + 8)
    }
  })

  it('max-md hit wrap is vertical-only 44px and has no debug outline', () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate() },
      global: { plugins: [i18n] },
    })
    const wrap = w.get('[data-testid="gate-share-hit-wrap"]')
    const wrapCls = wrap.classes().join(' ')
    expect(wrapCls).toContain('max-md:min-h-[44px]')
    expect(wrapCls).toContain('max-md:min-w-[44px]')
    expect(wrapCls).toContain('max-md:py-2.5')
    expect(wrapCls).toContain('max-md:-my-2.5')
    expect(wrapCls).not.toMatch(/dashed|outline-dashed|border-dashed/)
    expect(wrap.html()).not.toMatch(/hit-ghost|show-hit/)
    const box = (wrap.element as HTMLElement).getBoundingClientRect()
    if (box.width > 0 && box.height > 0) {
      expect(box.width).toBeGreaterThanOrEqual(44)
      expect(box.height).toBeGreaterThanOrEqual(44)
    }
  })

  it('hit wrap click opens share when enabled', async () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate() },
      global: { plugins: [i18n] },
    })
    await w.get('[data-testid="gate-share-hit-wrap"]').trigger('click')
    expect(w.emitted('open-share')).toHaveLength(1)
    expect(w.emitted('select')).toBeFalsy()
  })

  it('chip click does not open share panel', async () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate() },
      global: { plugins: [i18n] },
    })
    await w.get('[data-testid="gate-share-status"]').trigger('click')
    expect(w.emitted('open-share')).toBeFalsy()
  })

  it('disables copy when used', async () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate({ shareLink: { state: 'used', canCreate: false } }) },
      global: { plugins: [i18n] },
    })
    expect(w.get('[data-testid="gate-share-status"]').text()).toContain('已使用')
    expect((w.get('[data-testid="gate-share-copy-btn"]').element as HTMLButtonElement).disabled).toBe(true)
    await w.get('[data-testid="gate-share-hit-wrap"]').trigger('click')
    expect(w.emitted('open-share')).toBeFalsy()
  })

  it('hit wrap does not open share when card is disabled', async () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate(), disabled: true },
      global: { plugins: [i18n] },
    })
    expect((w.get('[data-testid="gate-share-copy-btn"]').element as HTMLButtonElement).disabled).toBe(true)
    await w.get('[data-testid="gate-share-hit-wrap"]').trigger('click')
    expect(w.emitted('open-share')).toBeFalsy()
  })

  it('shows remaining time for active links', () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate({ shareLink: { state: 'active', remainingSec: 3600, canManage: true } }) },
      global: { plugins: [i18n] },
    })
    expect(w.get('[data-testid="gate-share-status"]').text()).toMatch(/有效/)
  })
})

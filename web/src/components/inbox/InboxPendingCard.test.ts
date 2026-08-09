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

function clarify(): ClarifyInboxItem {
  return {
    type: 'clarify',
    runId: 'run-2',
    nodeId: 'react1',
    workflowName: 'wf',
    label: '澄清需求',
    done: false,
    requestedAt: '2026-08-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
  }
}

describe('InboxPendingCard share entry', () => {
  it('shows copy button and status for human_gate only', async () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate() },
      global: { plugins: [i18n] },
    })
    expect(w.get('[data-testid="gate-share-copy-btn"]').text()).toContain('复制临时链接')
    expect(w.get('[data-testid="gate-share-status"]').text()).toContain('尚未创建')
    await w.get('[data-testid="gate-share-copy-btn"]').trigger('click')
    expect(w.emitted('open-share')).toHaveLength(1)

    const c = mount(InboxPendingCard, {
      props: { item: clarify() },
      global: { plugins: [i18n] },
    })
    expect(c.find('[data-testid="gate-share-copy-btn"]').exists()).toBe(false)
    expect(c.find('[data-testid="gate-share-status"]').exists()).toBe(false)
  })

  it('disables copy when used', () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate({ shareLink: { state: 'used', canCreate: false } }) },
      global: { plugins: [i18n] },
    })
    expect(w.get('[data-testid="gate-share-status"]').text()).toContain('已使用')
    expect((w.get('[data-testid="gate-share-copy-btn"]').element as HTMLButtonElement).disabled).toBe(true)
  })

  it('shows remaining time for active links', () => {
    const w = mount(InboxPendingCard, {
      props: { item: gate({ shareLink: { state: 'active', remainingSec: 3600, canManage: true } }) },
      global: { plugins: [i18n] },
    })
    expect(w.get('[data-testid="gate-share-status"]').text()).toMatch(/有效/)
  })
})

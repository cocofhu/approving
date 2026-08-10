import '../src/styles/global.css'
import { computed, createApp, defineComponent, h, provide, ref } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import { setTheme } from '../src/lib/theme'
import InboxPendingCard from '../src/components/inbox/InboxPendingCard.vue'
import GateShareLinkPanel from '../src/components/run/GateShareLinkPanel.vue'
import PublicGateApprovalView from '../src/views/PublicGateApprovalView.vue'
import { api } from '../src/lib/api'
import { rememberShareUrl } from '../src/lib/gateShareLink'
import type { ClarifyInboxItem, GateInboxItem, InboxItem } from '../src/lib/types'
import HtmlPreview from '../src/components/ui/HtmlPreview.vue'

installIdleScrollbar()
setTheme('dark')

const TOKEN = 'ab'.repeat(32)
const SHARE_URL = `http://127.0.0.1:5174/public/gate-approvals#t=${TOKEN}`

let reviewPreviewBusy = false
let reviewPreviewTurns: Array<{ role: string; text: string; at: string }> = [
  { role: 'agent', text: '请复审 research.json', at: '2026-08-01T00:00:00Z' },
]

;(window as unknown as { __idleReview?: () => void }).__idleReview = () => {
  reviewPreviewBusy = false
  reviewPreviewTurns = [
    { role: 'agent', text: '请复审 research.json', at: '2026-08-01T00:00:00Z' },
    { role: 'human', text: '请改标题', at: '2026-08-01T00:02:00Z' },
  ]
}

;(api as any).createGateShareLink = async (_runId: string, _nodeId: string, ttlTier = '24h') => ({
  id: 'gsl-e2e',
  url: SHARE_URL,
  ttlTier,
  expiresAt: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
  state: 'active',
})
;(api as any).regenGateShareLink = async () => ({
  id: 'gsl-e2e-2',
  url: SHARE_URL.replace(TOKEN, 'cd'.repeat(32)),
  ttlTier: '24h',
  expiresAt: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
  state: 'active',
})
;(api as any).revokeGateShareLink = async () => ({ status: 'revoked' })
;(api as any).createReviewShareLink = async (_runId: string, _nodeId: string, ttlTier = '24h') => ({
  id: 'gsl-e2e-review',
  url: SHARE_URL,
  ttlTier,
  expiresAt: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
  state: 'active',
})
;(api as any).regenReviewShareLink = async () => ({
  id: 'gsl-e2e-review-2',
  url: SHARE_URL.replace(TOKEN, 'cd'.repeat(32)),
  ttlTier: '24h',
  expiresAt: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
  state: 'active',
})
;(api as any).revokeReviewShareLink = async () => ({ status: 'revoked' })

const originalFetch = window.fetch.bind(window)
window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  if (url.includes('/public/gate-approvals/preview')) {
    const scene = new URLSearchParams(location.search).get('scene') || ''
    if (scene === 'public-review') {
      return new Response(
        JSON.stringify({
          status: 'active',
          kind: 'review',
          title: '调研',
          description: '待复审脱敏摘要',
          remainingSec: 3600,
          nonce: 'nonce-e2e-review',
          reactSessionAlive: true,
          sessionBusy: reviewPreviewBusy,
          waiting: reviewPreviewBusy ? 0 : 0,
          activeItem: reviewPreviewBusy ? { text: '请改标题' } : undefined,
          productKind: 'structured',
          productName: 'research.json',
          actions: { confirm: 'confirm', reply: 'reply', cancel: 'cancel' },
          structured: { name: 'research.json', title: '调研摘要', doc: { title: '调研摘要' } },
          turns: reviewPreviewTurns,
          upstream: { name: 'clarified_requirement.json', title: '澄清', summary: '已有澄清需求文档，可对照审阅当前主产物' },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    }
    return new Response(
      JSON.stringify({
        status: 'active',
        kind: 'human_gate',
        title: '审阅视觉稿',
        description: '请审阅脱敏产物',
        remainingSec: 3600,
        nonce: 'nonce-e2e',
        reactSessionAlive: true,
        productKind: 'visual',
        productName: 'page.html',
        actions: { approve: 'approve', reject: 'revise', confirm: 'approve', reply: 'reply', cancel: 'cancel' },
        visualHtml: '<p>ok</p>',
        structured: { title: '外部一次审批', goals: ['g1'] },
        turns: [{ role: 'agent', text: '请审阅 page.html', at: '2026-08-01T00:00:00Z' }],
        upstream: { name: 'clarified_requirement.json', title: '澄清', summary: '已有澄清需求文档，可对照审阅当前主产物' },
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/public/gate-approvals/reply')) {
    reviewPreviewBusy = true
    return new Response(JSON.stringify({ status: 'accepted' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  if (url.includes('/public/gate-approvals/cancel')) {
    return new Response(JSON.stringify({ status: 'ok' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  if (url.includes('/public/gate-approvals/decide')) {
    if (new URLSearchParams(location.search).get('slowDecide')) {
      await new Promise((r) => setTimeout(r, 800))
    }
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    if (body.action === 'confirm') {
      reviewPreviewBusy = false
      return new Response(JSON.stringify({ status: 'confirmed', action: 'confirm' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    const name = String(body.name || '').trim()
    const comment = String(body.comment || '').trim()
    if (!name || !comment) {
      return new Response(JSON.stringify({ error: 'audit_required', message: '请填写姓名与意见后再提交' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    const status = body.action === 'revise' ? 'rejected' : 'approved'
    return new Response(JSON.stringify({ status, action: body.action }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  return originalFetch(input, init)
}

const gate: GateInboxItem = {
  type: 'gate',
  nodeType: 'human_gate',
  runId: 'run-e2e-share',
  nodeId: 'hg-e2e',
  iteration: 1,
  workflowName: 'wf',
  title: '审阅视觉稿',
  bodyMd: '请审阅',
  actions: [
    { id: 'approve', label: '批准' },
    { id: 'revise', label: '驳回', requireForm: true },
  ],
  requestedAt: '2026-08-01T00:00:00Z',
  shareLink: { state: 'none', canCreate: true, hasPass: true, hasFail: true },
}

const reviewItem: ClarifyInboxItem = {
  type: 'clarify',
  kind: 'review',
  runId: 'run-e2e-review',
  nodeId: 'research-e2e',
  iteration: 1,
  workflowName: 'wf',
  label: '调研',
  done: false,
  requestedAt: '2026-08-01T00:00:00Z',
  updatedAt: '2026-08-01T00:00:00Z',
  shareLink: { state: 'none', canCreate: true },
}

const ToolbarHost = defineComponent({
  name: 'ToolbarHost',
  setup() {
    provide('gateShareOpen', () => {})
    provide('gateShareEnabled', computed(() => true))
    return () =>
      h(HtmlPreview, {
        html: '<p>ok</p>',
        inspectable: true,
        enlargeable: true,
      })
  },
})

const Fixture = defineComponent({
  name: 'GateShareLinkFixture',
  setup() {
    const scene = new URLSearchParams(location.search).get('scene') || 'inbox'
    const open = ref(false)
    const item = ref<InboxItem>(scene === 'inbox-review' ? { ...reviewItem } : { ...gate })
    const kind = scene === 'inbox-review' ? 'review' : 'human_gate'

    if (scene === 'public' || scene === 'public-review') {
      if (!location.hash) location.hash = `#t=${TOKEN}`
      return () => h('div', { 'data-testid': 'gate-share-e2e-root' }, [h(PublicGateApprovalView)])
    }

    rememberShareUrl(item.value.runId, item.value.nodeId, item.value.iteration, '')
    return () =>
      h('div', { class: 'mx-auto max-w-md p-4', 'data-testid': 'gate-share-e2e-root' }, [
        h(InboxPendingCard, {
          item: item.value,
          active: true,
          onSelect: () => {},
          onOpenShare: () => {
            open.value = true
          },
        }),
        h(ToolbarHost),
        h(GateShareLinkPanel, {
          open: open.value,
          target: {
            runId: item.value.runId,
            nodeId: item.value.nodeId,
            iteration: item.value.iteration,
            shareLink: item.value.shareLink,
            kind,
          },
          kind,
          onClose: () => {
            open.value = false
          },
          onUpdated: (st: GateInboxItem['shareLink']) => {
            item.value = { ...item.value, shareLink: st }
          },
          onRevoked: (st: GateInboxItem['shareLink']) => {
            item.value = { ...item.value, shareLink: st }
            open.value = false
          },
        }),
      ])
  },
})

async function boot() {
  await initLocale()
  await setLocale('zh-CN')
  createApp(Fixture).use(i18n).mount('#app')
}
void boot()

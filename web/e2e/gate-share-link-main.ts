import '../src/styles/global.css'
import { createApp, defineComponent, h, ref } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import { setTheme } from '../src/lib/theme'
import InboxPendingCard from '../src/components/inbox/InboxPendingCard.vue'
import GateShareLinkPanel from '../src/components/run/GateShareLinkPanel.vue'
import PublicGateApprovalView from '../src/views/PublicGateApprovalView.vue'
import { api } from '../src/lib/api'
import { rememberShareUrl } from '../src/lib/gateShareLink'
import type { GateInboxItem } from '../src/lib/types'

installIdleScrollbar()
setTheme('dark')

const TOKEN = 'ab'.repeat(32)
const SHARE_URL = `http://127.0.0.1:5174/public/gate-approvals#t=${TOKEN}`

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

const originalFetch = window.fetch.bind(window)
window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  if (url.includes('/public/gate-approvals/preview')) {
    return new Response(
      JSON.stringify({
        status: 'active',
        title: '审阅视觉稿',
        description: '请审阅脱敏产物',
        remainingSec: 3600,
        nonce: 'nonce-e2e',
        actions: { approve: 'approve', reject: 'revise' },
        visualHtml: '<p>ok</p>',
        structured: { title: '外部一次审批', goals: ['g1'] },
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/public/gate-approvals/decide')) {
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    if (body.action === 'revise' && !String(body.comment || '').trim()) {
      return new Response(JSON.stringify({ error: 'comment_required', message: '驳回必须填写意见' }), {
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

const Fixture = defineComponent({
  name: 'GateShareLinkFixture',
  setup() {
    const scene = new URLSearchParams(location.search).get('scene') || 'inbox'
    const open = ref(false)
    const item = ref<GateInboxItem>({ ...gate })

    if (scene === 'public') {
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
        h(GateShareLinkPanel, {
          open: open.value,
          gate: item.value,
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

import type { ClarifyInboxItem, GateInboxItem, GateShareInboxStatus, InboxItem } from '@/lib/types'

export const GATE_SHARE_TTL_TIERS = ['1h', '8h', '24h', '72h', '7d'] as const
export type GateShareTTLTier = (typeof GATE_SHARE_TTL_TIERS)[number]
export const DEFAULT_GATE_SHARE_TTL: GateShareTTLTier = '24h'

export const GATE_SHARE_TOKEN_HEADER = 'X-Gate-Share-Token'
export const GATE_SHARE_REQUEST_HEADER = 'X-Gate-Share-Requested'

const SHARE_URL_STORAGE_PREFIX = 'approving.gateShareUrl.'

/** In-memory cache plus sessionStorage so refresh can copy the same active URL. */
const shareUrlMemory = new Map<string, string>()

export function shareMemoryKey(runId: string, nodeId: string, iteration?: number): string {
  return `${runId}:${nodeId}:${iteration ?? 0}`
}

function storageKey(runId: string, nodeId: string, iteration?: number): string {
  return SHARE_URL_STORAGE_PREFIX + shareMemoryKey(runId, nodeId, iteration)
}

export function rememberShareUrl(runId: string, nodeId: string, iteration: number | undefined, url: string) {
  const u = url.trim()
  if (!u) return
  const memKey = shareMemoryKey(runId, nodeId, iteration)
  shareUrlMemory.set(memKey, u)
  try {
    sessionStorage.setItem(storageKey(runId, nodeId, iteration), u)
  } catch {
    /* private mode / quota */
  }
}

export function recallShareUrl(runId: string, nodeId: string, iteration?: number): string {
  const mem = shareUrlMemory.get(shareMemoryKey(runId, nodeId, iteration))
  if (mem) return mem
  try {
    return sessionStorage.getItem(storageKey(runId, nodeId, iteration)) || ''
  } catch {
    return ''
  }
}

export function forgetShareUrl(runId: string, nodeId: string, iteration?: number) {
  shareUrlMemory.delete(shareMemoryKey(runId, nodeId, iteration))
  try {
    sessionStorage.removeItem(storageKey(runId, nodeId, iteration))
  } catch {
    /* ignore */
  }
}

export function isHumanGateInboxItem(item: InboxItem | null | undefined): item is GateInboxItem {
  return !!item && item.type === 'gate' && item.nodeType === 'human_gate'
}

export function isReviewInboxItem(item: InboxItem | null | undefined): item is ClarifyInboxItem {
  return !!item && item.type === 'clarify' && item.kind === 'review'
}

export function isAppPreviewInboxItem(item: InboxItem | null | undefined): item is ClarifyInboxItem {
  return !!item && item.type === 'clarify' && item.kind === 'app_preview'
}

/** Inbox share entry: human_gate, 待复审, or app_preview (review share API). */
export function isShareableInboxItem(item: InboxItem | null | undefined): boolean {
  return isHumanGateInboxItem(item) || isReviewInboxItem(item) || isAppPreviewInboxItem(item)
}

/** App preview and Inbox review both mint ShareLinkKindReview links. */
export function inboxShareKind(item: InboxItem | null | undefined): 'human_gate' | 'review' {
  return isReviewInboxItem(item) || isAppPreviewInboxItem(item) ? 'review' : 'human_gate'
}

const SHARE_API_ERROR_KEYS: Record<string, string> = {
  no_standard_action: 'pages.gatesInbox.share.errors.noStandardAction',
  not_human_gate: 'pages.gatesInbox.share.errors.notHumanGate',
  not_review_session: 'pages.gatesInbox.share.errors.notReviewSession',
  used_readonly: 'pages.gatesInbox.share.errors.usedReadonly',
  run_ended: 'pages.gatesInbox.share.errors.runEnded',
  gate_not_pending: 'pages.gatesInbox.share.errors.gateNotPending',
  review_not_pending: 'pages.gatesInbox.share.errors.reviewNotPending',
  review_busy: 'pages.gatesInbox.share.errors.reviewBusy',
  review_validation_failed: 'pages.gatesInbox.share.errors.reviewValidationFailed',
  invalid_ttl: 'pages.gatesInbox.share.errors.invalidTtl',
  not_active: 'pages.gatesInbox.share.errors.notActive',
  not_found: 'pages.gatesInbox.share.errors.notFound',
  share_failed: 'pages.gatesInbox.share.errors.shareFailed',
}

export function shareApiErrorMessage(
  err: unknown,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  const msg = err instanceof Error ? err.message : String(err || '')
  const code = msg.trim()
  const key = SHARE_API_ERROR_KEYS[code]
  if (key) return t(key)
  return msg || t('pages.gatesInbox.share.errors.shareFailed')
}

/** Mask fragment token so the management panel never shows plaintext by default. */
export function maskShareUrl(url: string): string {
  const i = url.indexOf('#t=')
  if (i < 0) return url
  return `${url.slice(0, i + 3)}••••••••`
}

/** Read token from `#t=…` only — never from path/query. */
export function parseShareTokenFromHash(hash: string): string {
  const raw = (hash || '').startsWith('#') ? hash.slice(1) : hash || ''
  if (!raw) return ''
  const params = new URLSearchParams(raw)
  return (params.get('t') || '').trim()
}

export function isGateShareActive(st?: GateShareInboxStatus | null): boolean {
  return st?.state === 'active'
}

export function canCreateGateShare(st?: GateShareInboxStatus | null): boolean {
  if (!st) return true
  if (st.state === 'used') return false
  if (st.canCreate != null) return !!st.canCreate
  return st.state === 'none' || st.state === 'revoked' || st.state === 'expired'
}

export function formatRemainingSec(
  sec: number | undefined,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  const n = Math.max(0, Math.floor(sec ?? 0))
  if (n <= 0) return t('pages.gatesInbox.share.expired')
  const days = Math.floor(n / 86400)
  if (days >= 2) return t('pages.gatesInbox.share.remainingDays', { n: days })
  const hours = Math.floor(n / 3600)
  if (hours >= 1) return t('pages.gatesInbox.share.remainingHours', { n: hours })
  const mins = Math.max(1, Math.floor(n / 60))
  return t('pages.gatesInbox.share.remainingMinutes', { n: mins })
}

export function shareStatusLabel(
  st: GateShareInboxStatus | undefined,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  const state = st?.state || 'none'
  if (state === 'active') {
    return t('pages.gatesInbox.share.stateActive', {
      remaining: formatRemainingSec(st?.remainingSec, t),
    })
  }
  if (state === 'used') return t('pages.gatesInbox.share.stateUsed')
  if (state === 'revoked') return t('pages.gatesInbox.share.stateRevoked')
  if (state === 'expired') return t('pages.gatesInbox.share.stateExpired')
  return t('pages.gatesInbox.share.stateNone')
}

export type PublicGatePreviewTurn = {
  role: 'agent' | 'human' | string
  text?: string
  at?: string
  interrupted?: boolean
  annotations?: Array<{
    selector?: string
    jsonPath?: string
    label?: string
    note?: string
    quote?: string
  }>
}

export type PublicGateQueueItem = {
  id?: string
  text?: string
}

export type PublicGateActiveItem = {
  id?: string
  text?: string
  annotations?: PublicGatePreviewTurn['annotations']
}

export type PublicGatePreview = {
  status: string
  kind?: 'human_gate' | 'review' | string
  title?: string
  description?: string
  remainingSec?: number
  expiresAt?: string
  actions?: { approve?: string; reject?: string; confirm?: string; reply?: string; cancel?: string }
  visualHtml?: string
  structured?: {
    name?: string
    title?: string
    goals?: unknown
    text?: string
    description?: string
    doc?: Record<string, unknown>
  }
  nonce?: string
  turns?: PublicGatePreviewTurn[]
  upstream?: {
    name?: string
    title?: string
    summary?: string
    description?: string
    text?: string
    doc?: Record<string, unknown>
  }
  reactSessionAlive?: boolean
  sessionBusy?: boolean
  waiting?: number
  queueItems?: PublicGateQueueItem[]
  activeItem?: PublicGateActiveItem | null
  productKind?: 'visual' | 'structured' | 'app_preview' | string
  productName?: string
}

export type PublicGateDecideResult = {
  status: string
  action?: string
  alreadyProcessed?: boolean
  error?: string
  message?: string
  kind?: string
}

export type PublicGateReplyResult = {
  status: string
  error?: string
  message?: string
  kind?: string
}

async function readJson<T>(res: Response): Promise<T> {
  try {
    return (await res.json()) as T
  } catch {
    throw new Error(`${res.status}`)
  }
}

/** Unauthenticated public gate APIs. Token never goes in path/query. */
export const publicGateApi = {
  preview(token: string, signal?: AbortSignal): Promise<PublicGatePreview> {
    return fetch('/public/gate-approvals/preview', {
      method: 'GET',
      credentials: 'omit',
      signal,
      headers: {
        [GATE_SHARE_TOKEN_HEADER]: token,
        [GATE_SHARE_REQUEST_HEADER]: '1',
      },
    }).then(async (res) => {
      const body = await readJson<PublicGatePreview & { error?: string; message?: string }>(res)
      if (!res.ok) {
        throw Object.assign(new Error(body.message || body.error || `${res.status}`), {
          status: res.status,
          body,
        })
      }
      return body
    })
  },
  decide(
    payload: {
      token: string
      action: string
      comment?: string
      name?: string
      nonce: string
    },
    signal?: AbortSignal,
  ): Promise<PublicGateDecideResult> {
    return fetch('/public/gate-approvals/decide', {
      method: 'POST',
      credentials: 'omit',
      signal,
      headers: {
        'Content-Type': 'application/json',
        [GATE_SHARE_REQUEST_HEADER]: '1',
      },
      body: JSON.stringify(payload),
    }).then(async (res) => {
      const body = await readJson<PublicGateDecideResult>(res)
      if (!res.ok && res.status !== 409) {
        throw Object.assign(new Error(body.message || body.error || `${res.status}`), {
          status: res.status,
          body,
        })
      }
      return { ...body, status: body.status || (res.status === 409 ? 'used' : body.status) }
    })
  },
  reply(payload: {
    token: string
    text: string
    annotations?: Array<{
      selector?: string
      jsonPath?: string
      label?: string
      note?: string
      quote?: string
    }>
    images?: Array<{ data?: string; mimeType?: string; name?: string }>
  }): Promise<PublicGateReplyResult> {
    return fetch('/public/gate-approvals/reply', {
      method: 'POST',
      credentials: 'omit',
      headers: {
        'Content-Type': 'application/json',
        [GATE_SHARE_REQUEST_HEADER]: '1',
      },
      body: JSON.stringify(payload),
    }).then(async (res) => {
      const body = await readJson<PublicGateReplyResult>(res)
      if (!res.ok) {
        throw Object.assign(new Error(body.message || body.error || `${res.status}`), {
          status: res.status,
          body,
        })
      }
      return body
    })
  },
  cancel(token: string): Promise<PublicGateReplyResult> {
    return fetch('/public/gate-approvals/cancel', {
      method: 'POST',
      credentials: 'omit',
      headers: {
        'Content-Type': 'application/json',
        [GATE_SHARE_REQUEST_HEADER]: '1',
      },
      body: JSON.stringify({ token }),
    }).then(async (res) => {
      const body = await readJson<PublicGateReplyResult>(res)
      if (!res.ok) {
        throw Object.assign(new Error(body.message || body.error || `${res.status}`), {
          status: res.status,
          body,
        })
      }
      return body
    })
  },
}

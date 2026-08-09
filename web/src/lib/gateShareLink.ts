import type { GateShareInboxStatus } from '@/lib/types'

export const GATE_SHARE_TTL_TIERS = ['1h', '8h', '24h', '72h', '7d'] as const
export type GateShareTTLTier = (typeof GATE_SHARE_TTL_TIERS)[number]
export const DEFAULT_GATE_SHARE_TTL: GateShareTTLTier = '24h'

export const GATE_SHARE_TOKEN_HEADER = 'X-Gate-Share-Token'
export const GATE_SHARE_REQUEST_HEADER = 'X-Gate-Share-Requested'

/** In-memory only (never persisted): last fragment URL for this gate instance. */
const shareUrlMemory = new Map<string, string>()

export function shareMemoryKey(runId: string, nodeId: string, iteration?: number): string {
  return `${runId}:${nodeId}:${iteration ?? 0}`
}

export function rememberShareUrl(runId: string, nodeId: string, iteration: number | undefined, url: string) {
  const u = url.trim()
  if (!u) return
  shareUrlMemory.set(shareMemoryKey(runId, nodeId, iteration), u)
}

export function recallShareUrl(runId: string, nodeId: string, iteration?: number): string {
  return shareUrlMemory.get(shareMemoryKey(runId, nodeId, iteration)) || ''
}

export function forgetShareUrl(runId: string, nodeId: string, iteration?: number) {
  shareUrlMemory.delete(shareMemoryKey(runId, nodeId, iteration))
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

export type PublicGatePreview = {
  status: string
  title?: string
  description?: string
  remainingSec?: number
  expiresAt?: string
  actions?: { approve?: string; reject?: string }
  visualHtml?: string
  structured?: { name?: string; title?: string; goals?: unknown; text?: string }
  nonce?: string
}

export type PublicGateDecideResult = {
  status: string
  action?: string
  alreadyProcessed?: boolean
  error?: string
  message?: string
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
  preview(token: string): Promise<PublicGatePreview> {
    return fetch('/public/gate-approvals/preview', {
      method: 'GET',
      credentials: 'omit',
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
  decide(payload: {
    token: string
    action: string
    comment?: string
    name?: string
    nonce: string
  }): Promise<PublicGateDecideResult> {
    return fetch('/public/gate-approvals/decide', {
      method: 'POST',
      credentials: 'omit',
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
}

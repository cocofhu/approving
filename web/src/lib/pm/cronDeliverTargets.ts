/** One recent cron delivery target derived from a channel PM thread. */
export type PushTargetOption = {
  /** scene:conversationId (no channel-type prefix) */
  value: string
  /** Thread title; may be empty — UI falls back to value */
  title: string
  updatedAt: string
  channelType?: string
  unspoken?: boolean
}

type ThreadLike = {
  userId: string
  title?: string
  updatedAt: string
  unspoken?: boolean
}

const VALID_SCENES = new Set(['c2c', 'group', 'guild'])
const CHANNEL_TYPES = new Set(['qq', 'wecom', 'feishu'])

export function parseChannelUserId(userId: string): { type: string; remainder: string } | null {
  const uid = String(userId || '')
  const colon = uid.indexOf(':')
  if (colon <= 0) return null
  const type = uid.slice(0, colon)
  const remainder = uid.slice(colon + 1)
  if (!CHANNEL_TYPES.has(type) || !remainder) return null
  return { type, remainder }
}

export function isChannelThreadUserId(userId: string | undefined | null): boolean {
  return !!parseChannelUserId(String(userId || ''))
}

/** True when value is scene:conversationId with a known scene and non-empty id. */
function isValidPushTargetValue(value: string): boolean {
  const colon = value.indexOf(':')
  if (colon <= 0 || colon === value.length - 1) return false
  const scene = value.slice(0, colon)
  const conversationId = value.slice(colon + 1)
  return VALID_SCENES.has(scene) && conversationId.length > 0
}

/**
 * Derive up to 10 recent push targets from PM threads:
 * - keep only userId matching the editing Channel type prefix
 * - value = userId without that prefix
 * - keep only legal scene:conversationId (scene ∈ {c2c,group,guild})
 * - dedupe by value, keep the newest updatedAt
 * - sort by updatedAt descending, take first 10
 */
export function deriveRecentPushTargets(
  threads: ThreadLike[],
  channelType: string = 'qq',
): PushTargetOption[] {
  const best = new Map<string, PushTargetOption>()
  for (const thr of threads) {
    const parsed = parseChannelUserId(String(thr.userId || ''))
    if (!parsed) continue
    if (parsed.type !== channelType) continue
    const value = parsed.remainder
    if (!isValidPushTargetValue(value)) continue
    const next: PushTargetOption = {
      value,
      title: String(thr.title || ''),
      updatedAt: String(thr.updatedAt || ''),
      channelType: parsed.type,
      unspoken: !!thr.unspoken,
    }
    const prev = best.get(value)
    if (!prev || next.updatedAt > prev.updatedAt) {
      best.set(value, next)
    }
  }
  return [...best.values()]
    .sort((a, b) => (a.updatedAt < b.updatedAt ? 1 : a.updatedAt > b.updatedAt ? -1 : 0))
    .slice(0, 10)
}

export function pushTargetPrimaryLabel(opt: Pick<PushTargetOption, 'value' | 'title' | 'unspoken'>): string {
  const title = String(opt.title || '').trim()
  const base = title || opt.value
  return opt.unspoken ? `${base} · 未发言` : base
}

export function channelPeerId(userId: string): string {
  const parsed = parseChannelUserId(userId)
  if (!parsed) return ''
  const parts = parsed.remainder.split(':')
  return parts.slice(1).join(':')
}

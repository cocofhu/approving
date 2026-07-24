/** One recent cron delivery target derived from a QQ channel PM thread. */
export type PushTargetOption = {
  /** scene:conversationId (no qq: prefix) */
  value: string
  /** Thread title; may be empty — UI falls back to value */
  title: string
  updatedAt: string
}

type ThreadLike = {
  userId: string
  title?: string
  updatedAt: string
}

const VALID_SCENES = new Set(['c2c', 'group', 'guild'])

/** True when value is scene:conversationId with a known scene and non-empty id. */
export function isValidPushTargetValue(value: string): boolean {
  const colon = value.indexOf(':')
  if (colon <= 0 || colon === value.length - 1) return false
  const scene = value.slice(0, colon)
  const conversationId = value.slice(colon + 1)
  return VALID_SCENES.has(scene) && conversationId.length > 0
}

/**
 * Derive up to 10 recent push targets from PM threads:
 * - keep only userId starting with `qq:`
 * - value = userId without the `qq:` prefix
 * - keep only legal scene:conversationId (scene ∈ {c2c,group,guild})
 * - dedupe by value, keep the newest updatedAt
 * - sort by updatedAt descending, take first 10
 */
export function deriveRecentPushTargets(threads: ThreadLike[]): PushTargetOption[] {
  const best = new Map<string, PushTargetOption>()
  for (const thr of threads) {
    const uid = String(thr.userId || '')
    if (!uid.startsWith('qq:')) continue
    const value = uid.slice(3)
    if (!isValidPushTargetValue(value)) continue
    const next: PushTargetOption = {
      value,
      title: String(thr.title || ''),
      updatedAt: String(thr.updatedAt || ''),
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

export function pushTargetPrimaryLabel(opt: Pick<PushTargetOption, 'value' | 'title'>): string {
  const title = String(opt.title || '').trim()
  return title || opt.value
}

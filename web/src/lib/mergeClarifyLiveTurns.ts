import type { ClarifyTurn } from '@/lib/types'

/**
 * Merge persisted transcript with in-flight liveTurns without duplicating
 * human/agent bubbles that softRefresh/loadRun already wrote into props.turns.
 *
 * Finds the longest prefix of `live` that matches the suffix of `persisted`
 * by role+text (streaming turns never match persisted), then appends only the
 * unmatched live tail (typically the streaming agent).
 */
export function mergePersistedAndLiveTurns(
  persisted: ClarifyTurn[],
  live: ClarifyTurn[],
): ClarifyTurn[] {
  if (!live.length) return persisted
  if (!persisted.length) return [...persisted, ...live]

  const maxK = Math.min(live.length, persisted.length)
  let matched = 0
  for (let candidate = maxK; candidate >= 1; candidate--) {
    let ok = true
    for (let i = 0; i < candidate; i++) {
      const lt = live[i]
      if (lt.streaming) {
        ok = false
        break
      }
      const pt = persisted[persisted.length - candidate + i]
      if (pt.role !== lt.role || (pt.text || '') !== (lt.text || '')) {
        ok = false
        break
      }
    }
    if (ok) {
      matched = candidate
      break
    }
  }
  if (matched === 0) return [...persisted, ...live]
  return [...persisted, ...live.slice(matched)]
}

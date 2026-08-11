import { mergeAcpEvents, type MergedAcpEvent } from './mergeAcpEvents'
import type { AcpEvent } from '../shared/types'

/** Event-page shape shared by RunDetailView reactive state and session cache. */
export type LiveWsEventPage = {
  events: MergedAcpEvent[]
  nextCursor: string
  hasMore: boolean
  live: boolean
}

/**
 * Apply a live WS ACP frame onto an existing page snapshot.
 *
 * Returns `null` when the frame must not rewrite the timeline (busy-only /
 * `events=[]`). Callers should still update busy/liveNode for those frames,
 * but must not sync an empty page into the session cache.
 *
 * When `hasMore` is false the whole page is the live tail, matching
 * `mergeAcpEvents` empty-frame preservation (prefix slicing would drop prev).
 */
export function applyLiveWsAcpPage(
  prev: LiveWsEventPage | undefined,
  wsEvents: AcpEvent[],
): LiveWsEventPage | null {
  // Empty live frames must not wipe a settled timeline or overwrite cache.
  if (wsEvents.length === 0) return null

  if (!prev) {
    return {
      events: mergeAcpEvents([], wsEvents, { live: true }),
      nextCursor: '',
      hasMore: false,
      live: true,
    }
  }

  const prefixLen = Math.max(0, prev.events.length - wsEvents.length)
  if (prev.hasMore) {
    const prefix = prev.events.slice(0, prefixLen)
    const tailPrev = prev.events.slice(prefixLen)
    const mergedTail = mergeAcpEvents(tailPrev, wsEvents, { live: true })
    return {
      ...prev,
      events: [...prefix, ...mergedTail],
      live: true,
    }
  }

  // Full page is the live window — do not slice prev to empty before merge.
  const merged = mergeAcpEvents(prev.events, wsEvents, { live: true })
  return {
    ...prev,
    events: merged,
    live: true,
  }
}

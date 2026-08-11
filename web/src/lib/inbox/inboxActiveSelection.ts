import type { InboxItem } from '@/lib/shared/types'

/** Stable key for inbox list membership (matches usePendingGates.itemKey). */
export function inboxItemKey(it: Pick<InboxItem, 'runId' | 'nodeId'>): string {
  return `${it.runId}:${it.nodeId}`
}

/** Triple used for inbox-context fetch / processed short-circuit. */
export function inboxTripleKey(
  it: Pick<InboxItem, 'runId' | 'nodeId' | 'iteration'>,
): string {
  return `${it.runId}:${it.nodeId}:${it.iteration ?? 1}`
}

/**
 * After removing `removedKey` from `prevList`, pick the neighbor active item:
 * prefer the next item at the same index; else the previous; empty → null.
 */
export function pickNextActiveAfterRemove<T extends Pick<InboxItem, 'runId' | 'nodeId'>>(
  prevList: T[],
  removedKey: string,
  keyOf: (it: T) => string = inboxItemKey,
): T | null {
  const idx = prevList.findIndex((it) => keyOf(it) === removedKey)
  if (idx < 0) {
    return prevList[0] || null
  }
  const without = prevList.filter((_, i) => i !== idx)
  if (!without.length) return null
  // After removal, the old "next" slides into idx; else take the new last (old prev).
  return without[idx] || without[idx - 1] || null
}

/** True when an inbox-context error means the item left pending. */
export function isInboxLeftPendingError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err ?? '')
  return /no pending inbox item/i.test(msg)
}

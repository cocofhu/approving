/** PM Leader consult turn failure kinds (product-level). */
export type PmFailKind = 'connection' | 'sandbox' | 'empty' | 'unknown' | 'stopped'

export const PM_FAIL_KINDS: readonly PmFailKind[] = [
  'connection',
  'sandbox',
  'empty',
  'unknown',
  'stopped',
] as const

export function isPmFailKind(kind: string): kind is PmFailKind {
  return (PM_FAIL_KINDS as readonly string[]).includes(kind)
}

/** Map thrown errors / AbortError onto a product failKind. */
export function classifyPmTurnError(e: unknown): PmFailKind {
  const err = e as { name?: string; failKind?: string; message?: unknown } | null
  if (err?.name === 'AbortError') return 'stopped'
  const kind = err?.failKind
  if (kind && isPmFailKind(kind)) return kind
  const msg = String(err?.message ?? e ?? '').toLowerCase()
  if (msg.includes('sandbox timeout')) return 'sandbox'
  if (
    msg.includes('ws') ||
    msg.includes('websocket') ||
    msg.includes('connection') ||
    msg.includes('invalidstate')
  ) {
    return 'connection'
  }
  if (msg.includes('timeout')) return 'sandbox'
  return 'unknown'
}

type MsgLike = { id: string; role: string; status?: string }

/**
 * User turns with no following assistant reply and not already failed.
 * After refresh these are permanent orphans unless converged to a retryable fail card.
 */
export function findOrphanUserMessageIds(messages: MsgLike[]): string[] {
  const out: string[] = []
  for (let i = 0; i < messages.length; i++) {
    const m = messages[i]
    if (m.role !== 'user') continue
    if (m.status === 'failed') continue
    let hasAssistant = false
    for (let j = i + 1; j < messages.length; j++) {
      if (messages[j].role === 'user') break
      if (messages[j].role === 'assistant') {
        hasAssistant = true
        break
      }
    }
    if (!hasAssistant) out.push(m.id)
  }
  return out
}

/**
 * Orphans that should NOT be converged: when a draft covers that user turn,
 * or a live turn is in-process (skipAll). Converge default is connection.
 */
export function findConvergableOrphanIds(
  messages: MsgLike[],
  opts?: { draftUserMsgId?: string; skipAll?: boolean },
): string[] {
  if (opts?.skipAll) return []
  const orphans = findOrphanUserMessageIds(messages)
  if (!opts?.draftUserMsgId) return orphans
  return orphans.filter((id) => id !== opts.draftUserMsgId)
}

/**
 * ACP/WS fan-out dedup: only apply frames with seq strictly greater than the
 * last accepted seq. Frames without seq are treated as apply-once legacy.
 */
export function shouldApplyEventSeq(seq: unknown, lastSeq: number): boolean {
  if (typeof seq !== 'number' || !Number.isFinite(seq)) return true
  return seq > lastSeq
}

export function pmActiveThreadStorageKey(projectId: string): string {
  return `pm-leader:active-thread:${projectId}`
}

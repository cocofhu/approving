/**
 * Lightweight request-generation counter for discarding stale list/detail writes.
 * Pure seq semantics only — no UI state, skeleton, or progress slots.
 */
export function createListRequestSeq() {
  let seq = 0
  return {
    beginListRequest(): number {
      seq += 1
      return seq
    },
    isCurrentSeq(localSeq: number): boolean {
      return localSeq === seq
    },
    currentSeq(): number {
      return seq
    },
  }
}

export function httpStatusOf(err: unknown): number | undefined {
  if (typeof err !== 'object' || err == null || !('status' in err)) return undefined
  const status = (err as { status?: unknown }).status
  return typeof status === 'number' ? status : undefined
}

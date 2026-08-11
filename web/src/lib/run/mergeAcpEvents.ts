import type { AcpEvent } from '../shared/types'

export interface MergedAcpEvent extends AcpEvent {
  stableKey: string
}

function contentPrefix(e: AcpEvent): string {
  return (e.title || e.text || '').slice(0, 40)
}

function hashPrefix(s: string): string {
  let h = 0
  for (let i = 0; i < s.length; i++) {
    h = (Math.imul(31, h) + s.charCodeAt(i)) | 0
  }
  return (h >>> 0).toString(36)
}

function longer(a?: string, b?: string): string | undefined {
  if (!a) return b
  if (!b) return a
  return a.length >= b.length ? a : b
}

function lastThoughtIndex(events: readonly AcpEvent[]): number {
  for (let i = events.length - 1; i >= 0; i--) {
    if (events[i].kind === 'thought') return i
  }
  return -1
}

function settledKey(e: AcpEvent): string {
  return `${e.kind}:${e.t}:${hashPrefix(contentPrefix(e))}`
}

/** Attach stable keys; tail thought while live uses a fixed inflight slot. */
function withStableKeys(events: AcpEvent[], live: boolean): MergedAcpEvent[] {
  const lastThought = live ? lastThoughtIndex(events) : -1
  return events.map((e, i) => ({
    ...e,
    stableKey:
      live && i === lastThought && e.kind === 'thought'
        ? 'inflight:thought'
        : settledKey(e),
  }))
}

function tailInflightThought(prev: readonly MergedAcpEvent[]): MergedAcpEvent | null {
  if (!prev.length) return null
  const last = prev[prev.length - 1]
  if (last.stableKey === 'inflight:thought' || last.kind === 'thought') return last
  return null
}

/**
 * Merge two ACP event snapshots for WS + polling.
 * Incoming is authoritative for settled events; in-flight tail thought is
 * preserved when missing from incoming (text monotonically grows).
 */
export function mergeAcpEvents(
  prev: readonly MergedAcpEvent[],
  incoming: AcpEvent[],
  opts: { live?: boolean } = {},
): MergedAcpEvent[] {
  const live = opts.live ?? false
  // Live empty frames (busy-only / events=[]) must not wipe a settled timeline.
  // Return a shallow copy of prev; do not merge busy metadata on this path.
  if (live && incoming.length === 0) return [...prev]

  let merged = withStableKeys(incoming, live)

  if (!live || !prev.length) return merged

  const prevThought = tailInflightThought(prev)
  if (!prevThought) return merged

  const incThoughtIdx = lastThoughtIndex(merged)
  if (incThoughtIdx < 0) {
    merged = [...merged, { ...prevThought, stableKey: 'inflight:thought' }]
  } else {
    const inc = merged[incThoughtIdx]
    const atTail = incThoughtIdx === merged.length - 1
    merged[incThoughtIdx] = {
      ...inc,
      text: longer(prevThought.text, inc.text),
      stableKey: atTail ? 'inflight:thought' : settledKey(inc),
    }
  }

  return merged
}

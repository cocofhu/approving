import type { TokenUsage } from './types'

/** Sum of the four usage components. */
export function tokenUsageTotal(u: TokenUsage): number {
  return (
    (u.inputTokens || 0) +
    (u.outputTokens || 0) +
    (u.cacheReadTokens || 0) +
    (u.cacheWriteTokens || 0)
  )
}

/** Format token counts for display (en-US grouping). */
export function fmtTokenCount(n: number): string {
  return n.toLocaleString('en-US')
}

/**
 * token/s display aligned with Demo: ≥10 → one decimal, else two.
 * Returns null when rate is not computable (caller shows —).
 */
export function fmtTokenRate(totalTokens: number, wallSec: number): string | null {
  if (wallSec <= 0) return null
  const rate = totalTokens / wallSec
  return rate >= 10 ? rate.toFixed(1) : rate.toFixed(2)
}

export type TimelineUsageSummary = {
  /** null = no reported usage on any item (show — for total & rate). */
  totalTokens: number | null
  /** null = show — (no usage, or invalid wall denominator). */
  tokenRate: string | null
}

/**
 * Aggregate timeline items that have usage (incl. reported 0).
 * Items without usage are skipped; all-reported-zero yields total 0 (not —).
 */
export function summarizeTimelineUsage(
  items: { usage?: TokenUsage | null }[],
  wallSec: number,
): TimelineUsageSummary {
  let total = 0
  let hasAny = false
  for (const it of items) {
    if (it.usage == null) continue
    hasAny = true
    total += tokenUsageTotal(it.usage)
  }
  if (!hasAny) {
    return { totalTokens: null, tokenRate: null }
  }
  return {
    totalTokens: total,
    tokenRate: fmtTokenRate(total, wallSec),
  }
}

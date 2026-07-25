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

export type MultiRunUsageSummary = {
  /** null = no selected run reported usage (show — for Σ / avg / rate). */
  totalTokens: number | null
  /** Runs with at least one process that reported usage. */
  usageRunCount: number
  /** null when usageRunCount is 0; else round(Σ / usageRunCount). */
  avgTokens: number | null
  /** null when no total or wallSumSec ≤ 0. */
  tokenRate: string | null
}

/**
 * Multi-run token KPI: Σ and average only count runs with reported usage;
 * token/s uses the selected wall-clock sum (may include runs without usage).
 */
export function summarizeMultiRunUsage(
  perRunTotals: Array<number | null>,
  wallSumSec: number,
): MultiRunUsageSummary {
  let total = 0
  let usageRunCount = 0
  for (const t of perRunTotals) {
    if (t == null) continue
    usageRunCount += 1
    total += t
  }
  if (usageRunCount === 0) {
    return { totalTokens: null, usageRunCount: 0, avgTokens: null, tokenRate: null }
  }
  return {
    totalTokens: total,
    usageRunCount,
    avgTokens: Math.round(total / usageRunCount),
    tokenRate: fmtTokenRate(total, wallSumSec),
  }
}

/** Merge usage components; null/undefined entries are skipped (unreported ≠ 0). */
export function mergeTokenUsage(
  ...parts: Array<TokenUsage | null | undefined>
): TokenUsage | null {
  let hasAny = false
  const out: TokenUsage = {
    inputTokens: 0,
    outputTokens: 0,
    cacheReadTokens: 0,
    cacheWriteTokens: 0,
  }
  for (const u of parts) {
    if (u == null) continue
    hasAny = true
    out.inputTokens += u.inputTokens || 0
    out.outputTokens += u.outputTokens || 0
    out.cacheReadTokens += u.cacheReadTokens || 0
    out.cacheWriteTokens += u.cacheWriteTokens || 0
  }
  return hasAny ? out : null
}

/** totalTokens for one usage blob; null when unreported. */
export function totalTokensOrNull(usage: TokenUsage | null | undefined): number | null {
  if (usage == null) return null
  return tokenUsageTotal(usage)
}

/** Sum totalTokens where present; all-null → null (never coerce unreported to 0). */
export function sumTotalTokens(...values: Array<number | null | undefined>): number | null {
  let sum = 0
  let hasAny = false
  for (const v of values) {
    if (v == null) continue
    hasAny = true
    sum += v
  }
  return hasAny ? sum : null
}

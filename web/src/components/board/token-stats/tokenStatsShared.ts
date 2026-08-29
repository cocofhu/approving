/** Shared Token stats chart colors aligned with approved page.html Demo. */
export const TOKEN_PART_COLORS = {
  input: '#3b82f6',
  output: '#8b5cf6',
  cacheRead: '#14b8a6',
  cacheWrite: '#f59e0b',
} as const

/** Trend/rank source colors (workflow / pm share purple; PM distinguished by dashed line). */
export const TOKEN_SOURCE_COLORS = {
  workflow: '#6d5cff',
  pm: '#6d5cff',
} as const

export type TokenPartKey = keyof typeof TOKEN_PART_COLORS

export const TOKEN_PART_KEYS: TokenPartKey[] = ['input', 'output', 'cacheRead', 'cacheWrite']

export function clientTimezoneParams(): { timezone?: string; utcOffsetMinutes: number } {
  let timezone: string | undefined
  try {
    timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    timezone = undefined
  }
  const utcOffsetMinutes = -new Date().getTimezoneOffset()
  return { timezone, utcOffsetMinutes }
}

export function formatBucketLabel(bucket: string, bucketWidth: string): string {
  if (bucketWidth === 'hour') {
    // 2026-07-24T14 → 14:00 (distinct from day MM-DD / week Wxx)
    const hour = /^(\d{4})-(\d{2})-(\d{2})T(\d{2})$/.exec(bucket)
    if (hour) return `${hour[4]}:00`
    return bucket
  }
  if (bucketWidth === 'week') {
    // 2026-W30 → W30
    const m = /^(\d{4})-W(\d{2})$/.exec(bucket)
    if (m) return `W${m[2]}`
    return bucket
  }
  // 2026-07-25 → 07-25
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(bucket)
  if (m) return `${m[2]}-${m[3]}`
  return bucket
}

/** Demo「修复后」placeAfter: viewport pad / gap. */
export const TREND_TOOLTIP_PAD = 8
export const TREND_TOOLTIP_GAP = 10

/** Matches TREND_CHART_GRID (CHART_GRID left/right, containLabel:false). */
export const TREND_PLOT_LEFT = 42
export const TREND_PLOT_RIGHT = 12

/**
 * Map canvas-local x to a category index (boundaryGap:true, n equal slots).
 * x in the y-axis gutter or first slot snaps to 0 so a left-edge 0-value
 * point (07-11) stays hoverable in the E2E band x=40–96.
 */
export function trendCategoryIndexAtX(x: number, width: number, n: number): number {
  if (n <= 1) return 0
  const plotW = Math.max(1, width - TREND_PLOT_LEFT - TREND_PLOT_RIGHT)
  const idx = Math.floor((x - TREND_PLOT_LEFT) / (plotW / n))
  return Math.max(0, Math.min(n - 1, idx))
}

/**
 * Viewport placement for TokenTrendChart tooltip (Demo placeAfter).
 * Prefer above the caret; flip up↔down / left↔right on overflow, then clamp by real box size.
 */
export function placeTrendTooltipAfter(input: {
  caretX: number
  caretY: number
  tipW: number
  tipH: number
  vw?: number
  vh?: number
  pad?: number
  gap?: number
}): { left: number; top: number } {
  const pad = input.pad ?? TREND_TOOLTIP_PAD
  const gap = input.gap ?? TREND_TOOLTIP_GAP
  const vw = input.vw ?? (typeof window !== 'undefined' ? window.innerWidth : 0)
  const vh = input.vh ?? (typeof window !== 'undefined' ? window.innerHeight : 0)
  const { caretX, caretY, tipW, tipH } = input

  let top = caretY - gap - tipH
  if (top < pad) top = caretY + gap
  if (top + tipH > vh - pad) top = Math.max(pad, vh - pad - tipH)

  let left = caretX - tipW / 2
  if (left < pad) left = caretX + gap
  if (left + tipW > vw - pad) left = caretX - gap - tipW
  if (left < pad) left = pad
  if (left + tipW > vw - pad) left = vw - pad - tipW

  return { left, top }
}

/** ECharts tooltip.position: container-relative, viewport-clamped via placeAfter. */
export function echartsTooltipPosition(
  point: number[],
  contentSize: number[],
  container: { left: number; top: number } | null,
): [number, number] {
  const caretX = (container?.left ?? 0) + (point[0] ?? 0)
  const caretY = (container?.top ?? 0) + (point[1] ?? 0)
  const placed = placeTrendTooltipAfter({
    caretX,
    caretY,
    tipW: contentSize[0] ?? 168,
    tipH: contentSize[1] ?? 72,
  })
  return [placed.left - (container?.left ?? 0), placed.top - (container?.top ?? 0)]
}

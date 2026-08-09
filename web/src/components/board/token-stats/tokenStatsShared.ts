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

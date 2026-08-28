// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import {
  formatBucketLabel,
  placeTrendTooltipAfter,
  echartsTooltipPosition,
  trendCategoryIndexAtX,
  TREND_TOOLTIP_GAP,
  TREND_TOOLTIP_PAD,
  TREND_PLOT_LEFT,
} from './tokenStatsShared'

describe('formatBucketLabel', () => {
  it('formats day buckets as MM-DD and week buckets as Wxx', () => {
    expect(formatBucketLabel('2026-07-11', 'day')).toBe('07-11')
    expect(formatBucketLabel('2026-W30', 'week')).toBe('W30')
  })
})

describe('placeTrendTooltipAfter (Demo placeAfter, g2.1–g2.4)', () => {
  const vw = 800
  const vh = 600
  const tipW = 168
  const tipH = 72

  function box(caretX: number, caretY: number, w = tipW, h = tipH) {
    return placeTrendTooltipAfter({ caretX, caretY, tipW: w, tipH: h, vw, vh })
  }

  it('measures real box with pad=8 gap=10 and prefers above + centered (g2.1/g2.2/g2.3)', () => {
    const pos = box(400, 300)
    expect(TREND_TOOLTIP_PAD).toBe(8)
    expect(TREND_TOOLTIP_GAP).toBe(10)
    expect(pos.top).toBe(300 - TREND_TOOLTIP_GAP - tipH)
    expect(pos.left).toBe(400 - tipW / 2)
  })

  it('flips below then clamps when above would leave the viewport (left-edge 0-value / high point) (g2.2)', () => {
    const leftZero = box(48, 18)
    expect(leftZero.top).toBe(18 + TREND_TOOLTIP_GAP)
    expect(leftZero.top).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
    expect(leftZero.top + tipH).toBeLessThanOrEqual(vh - TREND_TOOLTIP_PAD)

    const high = box(400, 12)
    expect(high.top).toBe(12 + TREND_TOOLTIP_GAP)
    expect(high.top + tipH).toBeLessThanOrEqual(vh - TREND_TOOLTIP_PAD)
  })

  it('flips left↔right then clamps so left-edge 07-11 and right-edge stay in viewport (g2.3)', () => {
    const left = box(40, 200)
    expect(left.left).toBe(40 + TREND_TOOLTIP_GAP)
    expect(left.left).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
    expect(left.left + tipW).toBeLessThanOrEqual(vw - TREND_TOOLTIP_PAD)

    const right = box(780, 200)
    expect(right.left + tipW).toBeLessThanOrEqual(vw - TREND_TOOLTIP_PAD)
    expect(right.left).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
  })

  it('keeps full box in viewport when tip is tall vs ~200px chart (no text truncation) (g2.4)', () => {
    const tallH = 160
    const pos = placeTrendTooltipAfter({
      caretX: 400,
      caretY: 120,
      tipW: 168,
      tipH: tallH,
      vw,
      vh: 220,
    })
    expect(pos.top).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
    expect(pos.top + tallH).toBeLessThanOrEqual(220 - TREND_TOOLTIP_PAD)
    expect(pos.left).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
    expect(pos.left + 168).toBeLessThanOrEqual(vw - TREND_TOOLTIP_PAD)
  })
})

describe('trendCategoryIndexAtX left-edge 0-value (g2.2 / g4.5)', () => {
  it('maps E2E hover band x=40–48 to index 0 (07-11) on typical canvas widths', () => {
    expect(TREND_PLOT_LEFT).toBe(42)
    for (const width of [390, 450, 900]) {
      expect(trendCategoryIndexAtX(40, width, 30)).toBe(0)
      expect(trendCategoryIndexAtX(48, width, 30)).toBe(0)
    }
    expect(trendCategoryIndexAtX(0, 450, 30)).toBe(0)
    expect(trendCategoryIndexAtX(450, 450, 30)).toBe(29)
  })
})

describe('echartsTooltipPosition (g2.2)', () => {
  it('keeps left-edge 07-11 tooltip inside the viewport after container offset', () => {
    const container = { left: 200, top: 120 }
    const [x, y] = echartsTooltipPosition([48, 172], [168, 72], container)
    const viewLeft = container.left + x
    const viewTop = container.top + y
    expect(viewLeft).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
    expect(viewTop).toBeGreaterThanOrEqual(TREND_TOOLTIP_PAD)
    expect(viewLeft + 168).toBeLessThanOrEqual((typeof window !== 'undefined' ? window.innerWidth : 1024) + 1)
    expect(viewTop + 72).toBeLessThanOrEqual((typeof window !== 'undefined' ? window.innerHeight : 768) + 1)
  })
})

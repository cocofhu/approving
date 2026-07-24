import { describe, expect, it } from 'vitest'
import {
  REVIEW_CANVAS_MIN,
  REVIEW_GAP,
  REVIEW_RIGHT_BUDGET,
  REVIEW_SIDEBAR,
  REVIEW_SHELL_WIDTH_KEY_APPROVAL,
  REVIEW_SHELL_WIDTH_KEY_REVIEW,
  REVIEW_STAGE_TARGET,
  computeDesktopReviewSplit,
  reviewRightPanelCssWidth,
} from './reviewLayoutBudget'

describe('reviewLayoutBudget constants', () => {
  it('matches approved page.html budget formula', () => {
    expect(REVIEW_STAGE_TARGET).toBe(480)
    expect(REVIEW_SIDEBAR).toBe(300)
    expect(REVIEW_GAP).toBe(1)
    expect(REVIEW_RIGHT_BUDGET).toBe(781)
    expect(REVIEW_CANVAS_MIN).toBe(200)
    expect(reviewRightPanelCssWidth()).toBe('min(781px, calc(100% - 200px))')
  })

  it('exposes distinct ReviewShell width storage keys', () => {
    expect(REVIEW_SHELL_WIDTH_KEY_REVIEW).toBe('review-shell-sidebar-width:review')
    expect(REVIEW_SHELL_WIDTH_KEY_APPROVAL).toBe('review-shell-sidebar-width:approval')
    expect(REVIEW_SHELL_WIDTH_KEY_REVIEW).not.toBe(REVIEW_SHELL_WIDTH_KEY_APPROVAL)
  })
})

describe('computeDesktopReviewSplit vs page.html viewports', () => {
  it('wide 1280: stage ≥480, canvas visible above floor', () => {
    const s = computeDesktopReviewSplit(1280)
    expect(s.right).toBe(781)
    expect(s.stage).toBeGreaterThanOrEqual(REVIEW_STAGE_TARGET)
    expect(s.canvas).toBeGreaterThan(REVIEW_CANVAS_MIN)
    expect(s.sidebar).toBe(300)
  })

  it('mid 980: canvas at floor, stage near target (may be 1px under)', () => {
    const s = computeDesktopReviewSplit(980)
    expect(s.canvas).toBe(REVIEW_CANVAS_MIN)
    expect(s.right).toBe(780)
    // availForRight 780 < budget 781 → slight compress; still ≫ 120px strip
    expect(s.stage).toBeGreaterThanOrEqual(REVIEW_STAGE_TARGET - REVIEW_GAP)
    expect(s.stage).toBeGreaterThan(120)
  })

  it('narrow 780: canvas ≥200, stage ≫120, still horizontal split room', () => {
    const s = computeDesktopReviewSplit(780)
    expect(s.canvas).toBe(REVIEW_CANVAS_MIN)
    expect(s.right).toBe(580)
    expect(s.stage).toBe(279)
    expect(s.stage).toBeGreaterThan(120)
    expect(s.stage).toBeLessThan(REVIEW_STAGE_TARGET)
    // stage + sidebar + gap fits in right panel
    expect(s.stage + s.sidebar + REVIEW_GAP).toBe(s.right)
  })

  it('degradation order: compress canvas to floor before stage dips under 480', () => {
    // Just above floor+budget: canvas still >200, stage at target.
    const wideEnough = computeDesktopReviewSplit(REVIEW_CANVAS_MIN + REVIEW_RIGHT_BUDGET + 50)
    expect(wideEnough.canvas).toBeGreaterThan(REVIEW_CANVAS_MIN)
    expect(wideEnough.stage).toBe(REVIEW_STAGE_TARGET)

    // Exactly at floor+budget: canvas pinned, stage still target.
    const atFloor = computeDesktopReviewSplit(REVIEW_CANVAS_MIN + REVIEW_RIGHT_BUDGET)
    expect(atFloor.canvas).toBe(REVIEW_CANVAS_MIN)
    expect(atFloor.stage).toBe(REVIEW_STAGE_TARGET)

    // Below floor+budget: canvas stays at floor, stage compresses but ≫120.
    const tight = computeDesktopReviewSplit(REVIEW_CANVAS_MIN + REVIEW_RIGHT_BUDGET - 100)
    expect(tight.canvas).toBe(REVIEW_CANVAS_MIN)
    expect(tight.stage).toBeLessThan(REVIEW_STAGE_TARGET)
    expect(tight.stage).toBeGreaterThan(120)
  })
})

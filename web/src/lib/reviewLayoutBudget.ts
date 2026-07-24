/**
 * Desktop Run Detail「复审」Tab layout budget.
 * Aligned with clarified_requirement + approved page.html prototype:
 * stage target 480 + sidebar 300 + gap 1 → right ≈ 781; canvas floor 200.
 */
export const REVIEW_STAGE_TARGET = 480
export const REVIEW_SIDEBAR = 300
export const REVIEW_GAP = 1
export const REVIEW_RIGHT_BUDGET = REVIEW_STAGE_TARGET + REVIEW_SIDEBAR + REVIEW_GAP
export const REVIEW_CANVAS_MIN = 200

/** localStorage keys for ReviewShell desktop sidebar width (scene-isolated). */
export const REVIEW_SHELL_WIDTH_KEY_REVIEW = 'review-shell-sidebar-width:review'
export const REVIEW_SHELL_WIDTH_KEY_APPROVAL = 'review-shell-sidebar-width:approval'

/** CSS width for the right panel when desktop review tab is active. */
export function reviewRightPanelCssWidth(
  budget = REVIEW_RIGHT_BUDGET,
  canvasMin = REVIEW_CANVAS_MIN,
): string {
  return `min(${budget}px, calc(100% - ${canvasMin}px))`
}

/**
 * Ideal split under the CSS rule width:min(budget, 100%-canvasMin)
 * with a flex-1 canvas (border-box, no extra chrome).
 */
export function computeDesktopReviewSplit(parentWidth: number) {
  const right = Math.min(REVIEW_RIGHT_BUDGET, parentWidth - REVIEW_CANVAS_MIN)
  const canvas = parentWidth - right
  const stage = right - REVIEW_SIDEBAR - REVIEW_GAP
  return { right, canvas, stage, sidebar: REVIEW_SIDEBAR }
}

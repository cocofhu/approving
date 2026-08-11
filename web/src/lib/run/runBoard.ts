import type { Run } from '@/lib/shared/types'

/** Short Run id for display (strip `run-` prefix). */
export function runIdShort(id: string): string {
  return id.replace(/^run-/, '')
}

/** Card / drawer title with stable fallbacks. */
export function runBoardTitle(run: Pick<Run, 'id' | 'title' | 'workflowName'>): string {
  const title = run.title?.trim()
  if (title) return title
  const wf = run.workflowName?.trim()
  if (wf) return wf
  return `#${runIdShort(run.id)}`
}

/** Sort key: startedAt DESC, then id DESC. Missing startedAt sorts last among peers. */
export function compareRunsByStartedAtDesc(a: Run, b: Run): number {
  const ta = a.startedAt ? Date.parse(a.startedAt) : NaN
  const tb = b.startedAt ? Date.parse(b.startedAt) : NaN
  const aValid = Number.isFinite(ta)
  const bValid = Number.isFinite(tb)
  if (aValid && bValid && ta !== tb) return tb - ta
  if (aValid && !bValid) return -1
  if (!aValid && bValid) return 1
  return b.id.localeCompare(a.id)
}

export function sortRunsByStartedAtDesc(runs: Run[]): Run[] {
  return [...runs].sort(compareRunsByStartedAtDesc)
}

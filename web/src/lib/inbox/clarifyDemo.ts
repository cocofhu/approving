import type { ReactOption, ReactQuestion } from '../shared/types'

/** Options that carry a non-empty demoHtml for iframe preview. */
export function demoOptionsOf(q: ReactQuestion): ReactOption[] {
  return q.options.filter((o) => !!o.demoHtml?.trim())
}

/** Side-by-side demo layout when at most three options have demos. */
export function useSideBySide(demoOptions: ReactOption[]): boolean {
  return demoOptions.length > 0 && demoOptions.length <= 3
}

/** Tailwind grid column class for 1–3 side-by-side demos (responsive ≤640px → single col). */
export function demoGridColsClass(count: number): string {
  if (count <= 1) return 'grid-cols-1'
  if (count === 2) return 'grid-cols-1 sm:grid-cols-2'
  return 'grid-cols-1 sm:grid-cols-3'
}

/** Match selected option labels from a parsed choice summary row. */
export function matchSelectedLabels(
  q: ReactQuestion,
  selectedLabels: string[],
): ReactOption[] {
  if (!selectedLabels.length) return []
  const set = new Set(selectedLabels)
  return q.options.filter((o) => set.has(o.label))
}

/** First selected option with demoHtml, for >3 select-preview mode. */
export function selectedDemoOption(
  q: ReactQuestion,
  selectedLabels: string[],
): ReactOption | null {
  const matched = matchSelectedLabels(q, selectedLabels)
  return matched.find((o) => !!o.demoHtml?.trim()) ?? null
}

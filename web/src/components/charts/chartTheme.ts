/** Shared ECharts axis / grid styling aligned with board token charts. */
export const CHART_AXIS = {
  axisLine: { show: false },
  axisTick: { show: false },
  axisLabel: { color: '#9aa1ad', fontSize: 10 },
  splitLine: { lineStyle: { color: '#eef0f3' } },
}

export const CHART_GRID = {
  left: 42,
  right: 12,
  top: 12,
  bottom: 28,
  containLabel: false,
}

export function fmtCompactAxis(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}

/** Shared ECharts axis / grid styling aligned with board token charts. */
export const CHART_AXIS = {
  axisLine: { show: false },
  axisTick: { show: false },
  axisLabel: { color: '#9aa1ad', fontSize: 10 },
  splitLine: { lineStyle: { color: '#eef0f3' } },
}

/** Dark board token panel: unified axis / grid / tooltip tones. */
export const BOARD_CHART_AXIS = {
  ...CHART_AXIS,
  axisLabel: { ...CHART_AXIS.axisLabel, color: '#9aa1ad' },
  splitLine: { lineStyle: { color: '#2a2e36' } },
}

export const BOARD_CHART_TOOLTIP = {
  borderRadius: 8,
  backgroundColor: '#1a1d23',
  borderColor: '#2a2e36',
  textStyle: { color: '#e8eaed', fontSize: 11 },
}

export const CHART_GRID = {
  left: 42,
  right: 12,
  top: 12,
  bottom: 28,
  containLabel: false,
}

export function fmtCompactAxis(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}

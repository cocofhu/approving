export type EdgeTone = 'ok' | 'err' | 'warn'

const VAR: Record<EdgeTone, string> = {
  ok: '--c-ok',
  err: '--c-err',
  warn: '--c-warn',
}

/** Read semantic stroke color from global.css tokens (supports theme switching). */
export function getEdgeStroke(tone: EdgeTone): string {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(VAR[tone]).trim()
  return raw ? `rgb(${raw})` : ''
}

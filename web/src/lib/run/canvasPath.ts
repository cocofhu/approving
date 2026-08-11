/** SVG path helpers for canvas edges — exported for unit tests. */

function sanitizeCoord(v: number): number {
  return Number.isFinite(v) ? v : 0
}

function sanitizePoint(x: number, y: number): [number, number] {
  return [sanitizeCoord(x), sanitizeCoord(y)]
}

/** Orthogonal polyline through the given points with rounded corners. */
export function roundedPolyline(pts: [number, number][], r: number): string {
  const clean: [number, number][] = []
  for (const [x, y] of pts) {
    const p = sanitizePoint(x, y)
    const prev = clean[clean.length - 1]
    if (prev && prev[0] === p[0] && prev[1] === p[1]) continue
    clean.push(p)
  }
  if (clean.length < 2) return ''
  let d = `M ${clean[0][0]},${clean[0][1]}`
  for (let i = 1; i < clean.length - 1; i++) {
    const [x0, y0] = clean[i - 1]
    const [x1, y1] = clean[i]
    const [x2, y2] = clean[i + 1]
    const l1 = Math.hypot(x1 - x0, y1 - y0)
    const l2 = Math.hypot(x2 - x1, y2 - y1)
    if (l1 === 0 || l2 === 0) continue
    const rr = Math.min(r, l1 / 2, l2 / 2)
    if (!Number.isFinite(rr) || rr <= 0) {
      d += ` L ${x1},${y1}`
      continue
    }
    const ax = x1 - ((x1 - x0) / l1) * rr
    const ay = y1 - ((y1 - y0) / l1) * rr
    const bx = x1 + ((x2 - x1) / l2) * rr
    const by = y1 + ((y2 - y1) / l2) * rr
    if (![ax, ay, bx, by, x1, y1].every(Number.isFinite)) {
      d += ` L ${x1},${y1}`
      continue
    }
    d += ` L ${ax},${ay} Q ${x1},${y1} ${bx},${by}`
  }
  const last = clean[clean.length - 1]
  d += ` L ${last[0]},${last[1]}`
  return d
}

export function fallbackLinePath(
  sx: number,
  sy: number,
  tx: number,
  ty: number,
): [string, number, number] {
  const [x1, y1] = sanitizePoint(sx, sy)
  const [x2, y2] = sanitizePoint(tx, ty)
  return [`M ${x1},${y1} L ${x2},${y2}`, (x1 + x2) / 2, (y1 + y2) / 2]
}

export function coordsFinite(...vals: number[]): boolean {
  return vals.every(Number.isFinite)
}

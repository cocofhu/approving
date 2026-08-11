import { describe, expect, it } from 'vitest'
import { coordsFinite, fallbackLinePath, roundedPolyline } from './canvasPath'

describe('roundedPolyline', () => {
  it('returns empty string for fewer than two distinct points', () => {
    expect(roundedPolyline([[0, 0]], 8)).toBe('')
    expect(roundedPolyline([[0, 0], [0, 0]], 8)).toBe('')
  })

  it('never emits NaN tokens in path d', () => {
    const d = roundedPolyline(
      [
        [0, 0],
        [NaN, NaN],
        [100, 50],
      ],
      10,
    )
    expect(d).not.toMatch(/NaN/)
    expect(d.startsWith('M ')).toBe(true)
  })

  it('skips degenerate zero-length segments', () => {
    const d = roundedPolyline(
      [
        [10, 10],
        [10, 10],
        [80, 10],
        [80, 60],
      ],
      8,
    )
    expect(d).not.toMatch(/NaN/)
    expect(d).toContain('80,60')
  })
})

describe('fallbackLinePath', () => {
  it('sanitizes non-finite coords to a straight segment', () => {
    const [d] = fallbackLinePath(NaN, 5, 20, NaN)
    expect(d).toBe('M 0,5 L 20,0')
    expect(d).not.toMatch(/NaN/)
  })
})

describe('coordsFinite', () => {
  it('detects finite coordinate tuples', () => {
    expect(coordsFinite(1, 2, 3, 4)).toBe(true)
    expect(coordsFinite(1, NaN, 3, 4)).toBe(false)
  })
})

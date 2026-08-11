import { describe, expect, it } from 'vitest'
import { computeSessionLayout, isInvalidPosition } from './canvasLayout'
import type { WFEdge, WFNode } from '../shared/types'

function node(id: string, type: WFNode['type'], pos: { x: number; y: number } | null, config: Record<string, unknown> = {}): WFNode {
  return { id, type, label: id, position: pos as { x: number; y: number }, config }
}

describe('isInvalidPosition', () => {
  it('treats null, undefined, and 0,0 as invalid', () => {
    expect(isInvalidPosition(null)).toBe(true)
    expect(isInvalidPosition(undefined)).toBe(true)
    expect(isInvalidPosition({ x: 0, y: 0 })).toBe(true)
  })

  it('keeps single-axis zero and non-zero coords valid', () => {
    expect(isInvalidPosition({ x: 0, y: 200 })).toBe(false)
    expect(isInvalidPosition({ x: 100, y: 0 })).toBe(false)
    expect(isInvalidPosition({ x: 36, y: 72 })).toBe(false)
  })
})

describe('computeSessionLayout', () => {
  it('returns empty map when all positions are valid', () => {
    const nodes = [node('a', 'input', { x: 10, y: 20 })]
    expect(computeSessionLayout(nodes, []).size).toBe(0)
  })

  it('assigns readable distinct coords to invalid nodes', () => {
    const nodes = [
      node('input', 'input', { x: 0, y: 0 }),
      node('react', 'react', { x: 0, y: 0 }),
      node('gate', 'human_gate', { x: 0, y: 0 }, {
        actions: [{ id: 'approve', goto: 'out' }, { id: 'revise', goto: 'react' }],
      }),
      node('out', 'visual', { x: 0, y: 0 }),
    ]
    const edges: WFEdge[] = [{ id: 'e1', source: 'input', target: 'react' }]
    const layout = computeSessionLayout(nodes, edges)
    expect(layout.size).toBe(4)
    const coords = [...layout.values()]
    const uniq = new Set(coords.map((c) => `${c.x},${c.y}`))
    expect(uniq.size).toBe(4)
  })

  it('does not reposition valid anchor nodes', () => {
    const nodes = [
      node('anchor', 'plan', { x: 400, y: 80 }),
      node('bad', 'implement', { x: 0, y: 0 }),
    ]
    const edges: WFEdge[] = [{ id: 'e1', source: 'anchor', target: 'bad' }]
    const layout = computeSessionLayout(nodes, edges)
    expect(layout.has('anchor')).toBe(false)
    expect(layout.get('bad')!.x).toBeGreaterThan(400)
  })
})

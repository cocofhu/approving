import { describe, expect, it } from 'vitest'
import { isApproveFirstPipeline, isPublishedApproveFirst, approveFirstNodeId } from './approveFirstPipeline'
import type { WFEdge, WFNode } from '@/lib/shared/types'

function graph(nodes: Partial<WFNode>[], edges: Partial<WFEdge>[]) {
  return {
    nodes: nodes.map((n) => ({
      id: String(n.id),
      type: n.type || 'react',
      label: n.label || String(n.id),
      position: { x: 0, y: 0 },
      config: {},
    })) as WFNode[],
    edges: edges.map((e, i) => ({
      id: e.id || `e${i}`,
      source: String(e.source),
      target: String(e.target),
      when: e.when,
      kind: e.kind,
    })) as WFEdge[],
  }
}

describe('isApproveFirstPipeline', () => {
  it('matches input → approve', () => {
    expect(
      isApproveFirstPipeline(
        graph(
          [
            { id: 'in', type: 'input' },
            { id: 'ap', type: 'approve' },
            { id: 'out', type: 'output' },
          ],
          [
            { source: 'in', target: 'ap' },
            { source: 'ap', target: 'out' },
          ],
        ),
      ),
    ).toBe(true)
  })

  it('rejects input → react', () => {
    expect(
      isApproveFirstPipeline(
        graph(
          [
            { id: 'in', type: 'input' },
            { id: 'r', type: 'react' },
            { id: 'out', type: 'output' },
          ],
          [{ source: 'in', target: 'r' }],
        ),
      ),
    ).toBe(false)
  })

  it('rejects graphs without an input node', () => {
    expect(
      isApproveFirstPipeline(
        graph([{ id: 'ap', type: 'approve' }, { id: 'out', type: 'output' }], [
          { source: 'ap', target: 'out' },
        ]),
      ),
    ).toBe(false)
  })

  it('uses the unguarded success edge when a when-guarded sibling exists', () => {
    expect(
      isApproveFirstPipeline(
        graph(
          [
            { id: 'in', type: 'input' },
            { id: 'ap', type: 'approve' },
            { id: 'r', type: 'react' },
          ],
          [
            { source: 'in', target: 'r', when: 'skip' },
            { source: 'in', target: 'ap' },
          ],
        ),
      ),
    ).toBe(true)
  })

  it('accepts a single success edge even when it has when', () => {
    expect(
      isApproveFirstPipeline(
        graph(
          [
            { id: 'in', type: 'input' },
            { id: 'ap', type: 'approve' },
          ],
          [{ source: 'in', target: 'ap', when: 'ok' }],
        ),
      ),
    ).toBe(true)
  })

  it('ignores failure edges leaving input', () => {
    expect(
      isApproveFirstPipeline(
        graph(
          [
            { id: 'in', type: 'input' },
            { id: 'ap', type: 'approve' },
            { id: 'fail', type: 'output' },
          ],
          [{ source: 'in', target: 'ap', kind: 'failure' }],
        ),
      ),
    ).toBe(false)
  })

  it('isPublishedApproveFirst requires published status', () => {
    const g = graph(
      [
        { id: 'in', type: 'input' },
        { id: 'ap', type: 'approve' },
      ],
      [{ source: 'in', target: 'ap' }],
    )
    expect(isPublishedApproveFirst({ status: 'draft', ...g })).toBe(false)
    expect(isPublishedApproveFirst({ status: 'published', ...g })).toBe(true)
    expect(approveFirstNodeId(g)).toBe('ap')
  })
})

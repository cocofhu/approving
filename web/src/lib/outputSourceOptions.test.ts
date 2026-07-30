import { describe, expect, it } from 'vitest'
import {
  buildOutputSourceOptions,
  labelForOutputTemplate,
  upstreamNodeIds,
} from './outputSourceOptions'
import type { WFEdge, WFNode } from './types'

const t = (key: string, params?: Record<string, unknown>) =>
  params?.value != null ? `${key}:${params.value}` : key

function node(id: string, type: string, label: string, config: Record<string, unknown> = {}): WFNode {
  return { id, type: type as never, label, position: { x: 0, y: 0 }, config } as WFNode
}

describe('outputSourceOptions', () => {
  const nodes: WFNode[] = [
    node('in', 'input', '输入'),
    node('plan', 'plan', '计划'),
    node('agent', 'agent', '实现', { produces: 'out.md' }),
    node('gate', 'human_gate', '门禁'),
  ]
  const edges: WFEdge[] = [
    { id: 'e1', source: 'in', target: 'plan' },
    { id: 'e2', source: 'plan', target: 'agent' },
    { id: 'e3', source: 'agent', target: 'gate' },
  ]

  it('walks transitive upstream ids', () => {
    expect([...upstreamNodeIds('gate', edges, nodes)].sort()).toEqual(['agent', 'in', 'plan'])
    expect(upstreamNodeIds('in', edges, nodes).size).toBe(0)
  })

  it('builds structured/agent/artifact options and resolves labels', () => {
    const opts = buildOutputSourceOptions(nodes, edges, 'gate', t)
    expect(opts.some((o) => o.value.includes('nodes.plan.outputs.plan'))).toBe(true)
    expect(opts.some((o) => o.value.includes('artifact("out.md")'))).toBe(true)
    expect(opts.some((o) => o.value.includes('nodes.agent.outputs.content'))).toBe(true)

    const hit = opts[0]
    expect(labelForOutputTemplate(hit.value, nodes, edges, 'gate', t)).toBe(hit.label)
    expect(labelForOutputTemplate('{{custom}}', nodes, edges, 'gate', t)).toContain(
      'common.gateBodyLabels.custom',
    )
  })

  it('includes upstream via test exit.goto only (no real edge to output)', () => {
    // plan_coverage: g1.1 / g3.1 — test|review exits.pass.goto
    const graph = [
      node('research', 'research', '调研'),
      node('test', 'test', '测试', {
        exits: { pass: { goto: 'output' }, fail: { goto: 'research' } },
      }),
      node('output', 'output', '输出'),
    ]
    const realEdges: WFEdge[] = [{ id: 'e1', source: 'research', target: 'test' }]
    const upstream = upstreamNodeIds('output', realEdges, graph)
    expect(upstream.has('test')).toBe(true)
    expect(upstream.has('research')).toBe(true)

    const opts = buildOutputSourceOptions(graph, realEdges, 'output', t)
    expect(opts.length).toBeGreaterThan(0)
    expect(opts.some((o) => o.value.includes('nodes.research.outputs.research'))).toBe(true)
    expect(opts.some((o) => o.value.includes('nodes.test.outputs.test_result'))).toBe(true)
  })

  it('includes upstream via review exit.goto only', () => {
    // plan_coverage: g3.1 — review exit.goto（仅第二段为 goto，无指向 output 的真实边）
    const graph = [
      node('implement', 'implement', '实现'),
      node('review', 'review', '评审', {
        exits: { pass: { goto: 'output' }, fail: { goto: 'implement' } },
      }),
      node('output', 'output', '输出'),
    ]
    const realEdges: WFEdge[] = [{ id: 'e1', source: 'implement', target: 'review' }]
    const opts = buildOutputSourceOptions(graph, realEdges, 'output', t)
    expect(opts.some((o) => o.value.includes('nodes.implement.outputs.implementation_result'))).toBe(
      true,
    )
    expect(opts.some((o) => o.value.includes('nodes.review.outputs.review'))).toBe(true)
  })

  it('includes upstream via human_gate action.goto only', () => {
    // plan_coverage: g1.1 / g3.1 — human_gate.actions[].goto
    const graph = [
      node('research', 'research', '调研'),
      node('gate', 'human_gate', '门禁', {
        actions: [{ id: 'approve', label: '通过', goto: 'output' }],
      }),
      node('output', 'output', '输出'),
    ]
    const realEdges: WFEdge[] = [{ id: 'e1', source: 'research', target: 'gate' }]
    const opts = buildOutputSourceOptions(graph, realEdges, 'output', t)
    expect(opts.some((o) => o.value.includes('nodes.research.outputs.research'))).toBe(true)
  })

  it('includes upstream via branch case.goto only', () => {
    // plan_coverage: g1.1 / g3.1 — branch.cases[].goto
    const graph = [
      node('research', 'research', '调研'),
      node('branch', 'branch', '分支', {
        cases: [{ label: 'ok', goto: 'output' }],
      }),
      node('output', 'output', '输出'),
    ]
    const realEdges: WFEdge[] = [{ id: 'e1', source: 'research', target: 'branch' }]
    const opts = buildOutputSourceOptions(graph, realEdges, 'output', t)
    expect(opts.some((o) => o.value.includes('nodes.research.outputs.research'))).toBe(true)
  })

  it('unions edges and goto, dedupes option templates', () => {
    // plan_coverage: g1.2 / g3.1 — edges ∪ goto 并集去重
    const graph = [
      node('implement', 'implement', '实现'),
      node('test', 'test', '测试', {
        exits: { pass: { goto: 'output' }, fail: { goto: 'implement' } },
      }),
      node('output', 'output', '输出'),
    ]
    const realEdges: WFEdge[] = [
      { id: 'e1', source: 'implement', target: 'test' },
      { id: 'e2', source: 'implement', target: 'output', kind: 'success' },
      { id: 'e3', source: 'test', target: 'output', kind: 'success' },
    ]
    const opts = buildOutputSourceOptions(graph, realEdges, 'output', t)
    const implValues = opts.filter((o) =>
      o.value.includes('nodes.implement.outputs.implementation_result'),
    )
    expect(implValues).toHaveLength(1)
    expect(opts.some((o) => o.value.includes('nodes.test.outputs.test_result'))).toBe(true)
  })

  it('keeps real-edge implement→output options (no regression)', () => {
    // plan_coverage: g1.3 / g3.1 — 真实边对照不回归
    const graph = [
      node('implement', 'implement', '实现'),
      node('output', 'output', '输出'),
    ]
    const realEdges: WFEdge[] = [
      { id: 'e1', source: 'implement', target: 'output', kind: 'success' },
    ]
    const opts = buildOutputSourceOptions(graph, realEdges, 'output', t)
    expect(opts.some((o) => o.value.includes('nodes.implement.outputs.implementation_result'))).toBe(
      true,
    )
  })
})

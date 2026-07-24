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
    expect([...upstreamNodeIds('gate', edges)].sort()).toEqual(['agent', 'in', 'plan'])
    expect(upstreamNodeIds('in', edges).size).toBe(0)
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
})

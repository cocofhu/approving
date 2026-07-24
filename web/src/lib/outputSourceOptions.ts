import type { WFEdge, WFNode } from './types'

export interface OutputSourceOption {
  value: string
  label: string
}

const STRUCTURED_OUT: Partial<Record<string, { key: string; labelKey: string }>> = {
  plan: { key: 'plan', labelKey: 'common.gateBodyLabels.plan' },
  react: { key: 'clarified_requirement', labelKey: 'common.gateBodyLabels.clarifiedRequirement' },
  research: { key: 'research', labelKey: 'common.gateBodyLabels.research' },
  proposal: { key: 'proposals', labelKey: 'common.gateBodyLabels.proposals' },
  proposal_select: { key: 'proposal', labelKey: 'common.gateBodyLabels.proposal' },
  test: { key: 'test_result', labelKey: 'common.gateBodyLabels.testResult' },
  review: { key: 'review', labelKey: 'common.gateBodyLabels.review' },
  implement: { key: 'implementation_result', labelKey: 'common.gateBodyLabels.implementationResult' },
  visual: { key: 'page', labelKey: 'common.gateBodyLabels.pagePreview' },
}

/** Collect upstream node ids reachable via edges (transitive predecessors). */
export function upstreamNodeIds(nodeId: string, edges: WFEdge[]): Set<string> {
  const preds: Record<string, string[]> = {}
  for (const e of edges) (preds[e.target] ||= []).push(e.source)
  const seen = new Set<string>()
  const stack = [...(preds[nodeId] || [])]
  while (stack.length) {
    const id = stack.pop()!
    if (seen.has(id)) continue
    seen.add(id)
    for (const p of preds[id] || []) stack.push(p)
  }
  return seen
}

/** Build selectable output source options for an output / human_gate node. */
export function buildOutputSourceOptions(
  allNodes: WFNode[],
  edges: WFEdge[],
  targetNodeId: string,
  t: (key: string, params?: Record<string, unknown>) => string,
): OutputSourceOption[] {
  const opts: OutputSourceOption[] = []
  const seen = new Set<string>()
  const add = (value: string, label: string) => {
    if (seen.has(value)) return
    seen.add(value)
    opts.push({ value, label })
  }
  const upstreamIds = upstreamNodeIds(targetNodeId, edges)
  for (const n of allNodes) {
    if (!upstreamIds.has(n.id)) continue
    const so = STRUCTURED_OUT[n.type]
    if (so) add(`{{nodes.${n.id}.outputs.${so.key}}}`, `${t(so.labelKey)} · ${n.label}`)
    const produces = (n.config?.produces || '').toString().trim()
    if (produces) add(`{{artifact("${produces}")}}`, `${t('common.gateBodyLabels.artifact')} · ${produces}`)
    if (n.type === 'agent') add(`{{nodes.${n.id}.outputs.content}}`, `${n.label} · ${t('common.gateBodyLabels.textOutput')}`)
  }
  return opts
}

/** Resolve a template value to its display label (for cards / custom sources). */
export function labelForOutputTemplate(
  template: string,
  allNodes: WFNode[],
  edges: WFEdge[],
  targetNodeId: string,
  t: (key: string, params?: Record<string, unknown>) => string,
): string {
  const opts = buildOutputSourceOptions(allNodes, edges, targetNodeId, t)
  const hit = opts.find((o) => o.value === template)
  if (hit) return hit.label
  return t('common.gateBodyLabels.custom', { value: template })
}

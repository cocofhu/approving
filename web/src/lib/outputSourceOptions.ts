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

function addPred(preds: Record<string, string[]>, source: string, target: string) {
  if (!source || !target || source === target) return
  ;(preds[target] ||= []).push(source)
}

/**
 * Collect upstream node ids reachable via real edges ∪ goto adjacency
 * (branch.cases / human_gate.actions / test|review exits), transitive.
 * Mirrors canvasLayout.buildAdjacency predecessor semantics for option discovery.
 */
export function upstreamNodeIds(nodeId: string, edges: WFEdge[], nodes: WFNode[] = []): Set<string> {
  const preds: Record<string, string[]> = {}
  for (const e of edges) addPred(preds, e.source, e.target)

  for (const n of nodes) {
    if (n.type === 'branch') {
      for (const c of (n.config?.cases as { goto?: string }[]) || []) {
        if (c?.goto) addPred(preds, n.id, c.goto)
      }
    }
    if (n.type === 'human_gate') {
      for (const a of (n.config?.actions as { id?: string; goto?: string }[]) || []) {
        if (a?.goto) addPred(preds, n.id, a.goto)
      }
    }
    if (n.type === 'test' || n.type === 'review') {
      const exits = (n.config?.exits as Record<string, { goto?: string }>) || {}
      for (const key of ['pass', 'fail']) {
        if (exits[key]?.goto) addPred(preds, n.id, exits[key].goto!)
      }
    }
  }

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
  const upstreamIds = upstreamNodeIds(targetNodeId, edges, allNodes)
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

import type { NodeRun, Run } from './types'

/** Last executed output node by startedAt (for auto-select on run complete). */
export function lastOutputNodeId(run: Run, nodes: { id: string; type: string }[]): string | null {
  const outputIds = new Set(nodes.filter((n) => n.type === 'output').map((n) => n.id))
  if (!outputIds.size) return null
  let bestId: string | null = null
  let bestAt = -1
  const consider = (nodeId: string, nr: NodeRun) => {
    if (!outputIds.has(nodeId)) return
    const at = nr.startedAt ? Date.parse(nr.startedAt) : 0
    if (at >= bestAt) {
      bestAt = at
      bestId = nodeId
    }
  }
  for (const [id, list] of Object.entries(run.nodeExecutions || {})) {
    for (const nr of list) consider(id, nr)
  }
  for (const [id, nr] of Object.entries(run.nodeRuns)) {
    consider(id, nr)
  }
  return bestId
}

/** First output node in graph order (fallback when nothing executed). */
export function firstGraphOutputNodeId(nodes: { id: string; type: string }[]): string | null {
  return nodes.find((n) => n.type === 'output')?.id ?? null
}

/**
 * Focus target for completed notify / live complete:
 * last executed output node, else first graph output node, else null.
 */
export function resolveOutputFocusNodeId(
  run: Run,
  nodes: { id: string; type: string }[],
): string | null {
  return lastOutputNodeId(run, nodes) || firstGraphOutputNodeId(nodes)
}

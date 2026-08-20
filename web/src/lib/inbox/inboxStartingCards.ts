import { isStartingInboxItem } from '@/lib/inbox/inboxDisplay'
import { clipRunTitle } from '@/lib/run/runTitle'
import type { ClarifyInboxItem, InboxItem, Run } from '@/lib/shared/types'

/** The approval a just-started run is expected to park on. */
export type IncomingApproval = { runId: string; nodeId: string }

/**
 * The approval the inbox is waiting for, from `?run=&node=` or the pending home
 * handoff. The published-graph node id may differ from the parked Approve id
 * (`ap` vs `approve_7gl8`), so nodeId is a hint and runId is the identity.
 */
export function resolveIncomingApproval(
  queryRun: unknown,
  queryNode: unknown,
  seed?: { runId?: string; nodeId?: string } | null,
): IncomingApproval | null {
  const runId = String(queryRun ?? '').trim() || seed?.runId || ''
  if (!runId) return null
  return { runId, nodeId: String(queryNode ?? '').trim() || seed?.nodeId || '' }
}

/** Optimistic loading row shown until listGates returns the real approval. */
export function makeIncomingGhost(
  target: IncomingApproval,
  seedText = '',
  now: string = new Date().toISOString(),
): ClarifyInboxItem {
  const label = clipRunTitle(seedText.trim()) || '…'
  return {
    type: 'clarify',
    state: 'starting',
    runId: target.runId,
    nodeId: target.nodeId || 'approve',
    iteration: 1,
    workflowName: '',
    runTitle: label,
    label,
    done: false,
    requestedAt: now,
    updatedAt: now,
  }
}

/**
 * Whether a starting approval never came up.
 *
 * A clarify/approve sandbox-setup failure records the failure on the node
 * execution and stops the run *without* marking the run terminal, so run status
 * alone would miss exactly the case this has to detect. A whole-run failure or
 * cancel still counts, and an unknown node id falls back to run status so a
 * guessed id cannot invent a failure.
 */
export function isStartFailedRun(
  run: Pick<Run, 'status'> & { nodeRuns?: Run['nodeRuns'] },
  nodeId: string,
): boolean {
  if (run.status === 'failed' || run.status === 'cancelled') return true
  const nodeStatus = nodeId ? run.nodeRuns?.[nodeId]?.status : undefined
  return nodeStatus === 'failed' || nodeStatus === 'cancelled'
}

/**
 * Starting rows present in `before` but gone from `after`. Each one still needs
 * a run check before it counts as a failure: a filter or page change drops live
 * rows too. `ignoreKey` skips the client-side ghost, which is not a server row.
 */
export function vanishedStartingRows(
  before: InboxItem[],
  after: InboxItem[],
  keyOf: (it: InboxItem) => string,
  ignoreKey = '',
): InboxItem[] {
  if (!before.length) return []
  const present = new Set(after.map(keyOf))
  return before.filter((it) => {
    if (!isStartingInboxItem(it)) return false
    const key = keyOf(it)
    return key !== ignoreKey && !present.has(key)
  })
}

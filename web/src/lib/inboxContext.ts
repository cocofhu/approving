import type { Artifact, ClarifyTurn, NodeRun, Run, WFNode } from './types'

export type GateInboxContext = {
  type: 'gate'
  nodes: WFNode[]
  artifacts: Artifact[]
  nodeExecutions: Record<string, NodeRun[]>
}

export type ClarifyInboxContext = {
  type: 'clarify'
  status: Run['status']
  /** Full graph nodes (same shape as gate inbox-context). */
  nodes?: WFNode[]
  /** Run-level artifact metadata (content loaded on demand). */
  artifacts?: Artifact[]
  /** Slim executions for current node ∪ parseable upstream refs. */
  nodeExecutions?: Record<string, NodeRun[]>
  clarify: {
    nodeId: string
    iteration?: number
    turns: ClarifyTurn[]
    done: boolean
    label: string
  }
}

export type InboxContextResponse = GateInboxContext | ClarifyInboxContext

/** Maps inbox-context API response to the Run subset GateApproval/ClarifyChat expect. */
export function adaptInboxContextToRun(ctx: InboxContextResponse, runId: string): Run {
  const base: Run = {
    id: runId,
    workflowId: '',
    workflowName: '',
    status: 'waiting_human',
    trigger: '',
    startedAt: '',
    durationSec: 0,
    progress: 0,
    nodeRuns: {},
    artifacts: [],
  }

  if (ctx.type === 'gate') {
    return {
      ...base,
      nodes: ctx.nodes,
      artifacts: ctx.artifacts,
      nodeExecutions: ctx.nodeExecutions,
    }
  }

  return {
    ...base,
    status: ctx.status,
    nodes: ctx.nodes,
    artifacts: ctx.artifacts ?? [],
    nodeExecutions: ctx.nodeExecutions,
    clarifyByNode: {
      [ctx.clarify.nodeId]: {
        nodeId: ctx.clarify.nodeId,
        iteration: ctx.clarify.iteration,
        turns: ctx.clarify.turns,
        done: ctx.clarify.done,
      },
    },
  }
}

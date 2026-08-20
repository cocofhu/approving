import { api } from '@/lib/api/api'
import type { Run } from '@/lib/shared/types'

export class ApproveParkTimeout extends Error {
  readonly runId: string
  constructor(runId: string) {
    super('approve park timeout')
    this.name = 'ApproveParkTimeout'
    this.runId = runId
  }
}

export function findApproveWaitingHuman(run: Pick<Run, 'status' | 'nodes' | 'nodeRuns'>): string | null {
  const nodes = run.nodes || []
  for (const n of nodes) {
    if (n.type !== 'approve') continue
    if (run.nodeRuns?.[n.id]?.status === 'waiting_human') return n.id
  }
  if (run.status === 'waiting_human') {
    const n = nodes.find((x) => x.type === 'approve')
    if (n) return n.id
  }
  return null
}

export async function waitForApprovePark(
  runId: string,
  opts?: {
    getRun?: (id: string) => Promise<Run>
    sleep?: (ms: number) => Promise<void>
    timeoutMs?: number
    intervalMs?: number
    signal?: AbortSignal
  },
): Promise<{ nodeId: string; run: Run }> {
  const getRun = opts?.getRun ?? ((id) => api.getRun(id))
  const sleep = opts?.sleep ?? ((ms) => new Promise((r) => setTimeout(r, ms)))
  const timeoutMs = opts?.timeoutMs ?? 30_000
  const intervalMs = opts?.intervalMs ?? 400
  const started = Date.now()
  while (true) {
    if (opts?.signal?.aborted) {
      throw new DOMException('Aborted', 'AbortError')
    }
    const run = await getRun(runId)
    if (run.status === 'failed' || run.status === 'cancelled') {
      throw new Error(run.error || run.failedReason || run.status)
    }
    const nodeId = findApproveWaitingHuman(run)
    if (nodeId) return { nodeId, run }
    if (Date.now() - started >= timeoutMs) throw new ApproveParkTimeout(runId)
    await sleep(intervalMs)
  }
}

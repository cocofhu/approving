import type { ClarifyImage } from '@/lib/shared/types'

export type HomeApproveHandoff = {
  runId: string
  nodeId: string
  text: string
  images: ClarifyImage[]
}

let current: HomeApproveHandoff | null = null

function cloneImages(images?: ClarifyImage[] | null): ClarifyImage[] {
  return (images || []).map((im) => ({ ...im }))
}

export function setHomeApproveHandoff(next: HomeApproveHandoff) {
  current = {
    runId: next.runId,
    nodeId: next.nodeId,
    text: next.text,
    images: cloneImages(next.images),
  }
}

export function takeHomeApproveHandoff(): HomeApproveHandoff | null {
  const got = current
  current = null
  return got
}

export function peekHomeApproveHandoff(): HomeApproveHandoff | null {
  return current
}

/** Same run; empty parked node id still belongs to this run (StartRun before graph resolve). */
function matchesHomeApproveHandoff(
  got: Pick<HomeApproveHandoff, 'runId' | 'nodeId'>,
  runId: string,
  nodeId: string,
): boolean {
  if (got.runId !== runId) return false
  if (got.nodeId && nodeId && got.nodeId !== nodeId) return false
  return true
}

/** Fill in the parked node id after waitForApprovePark, if Inbox has not consumed yet. */
export function updateHomeApproveHandoffNode(runId: string, nodeId: string): boolean {
  if (!current || !nodeId || current.runId !== runId) return false
  current = { ...current, nodeId }
  return true
}

/** Consume only when the parked card matches; otherwise leave the slot intact. */
export function consumeHomeApproveHandoff(runId: string, nodeId: string): HomeApproveHandoff | null {
  const got = current
  if (!got || !matchesHomeApproveHandoff(got, runId, nodeId)) return null
  current = null
  return { ...got, nodeId: got.nodeId || nodeId }
}

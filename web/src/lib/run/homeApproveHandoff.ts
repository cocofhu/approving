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

/**
 * Home chat always belongs to one new run. The published-graph node id may
 * differ from the parked Approve id (`ap` vs `approve_7gl8`), so match run only.
 */
export function homeApproveHandoffMatchesRun(
  got: Pick<HomeApproveHandoff, 'runId'>,
  runId: string,
): boolean {
  return !!got.runId && got.runId === runId
}

/** Consume only when the parked card matches; otherwise leave the slot intact. */
export function consumeHomeApproveHandoff(runId: string, nodeId: string): HomeApproveHandoff | null {
  const got = current
  if (!got || !homeApproveHandoffMatchesRun(got, runId)) return null
  current = null
  return { ...got, nodeId: nodeId || got.nodeId }
}

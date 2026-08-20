import type { ClarifyImage } from '@/lib/shared/types'

export type HomeApproveHandoff = {
  runId: string
  nodeId: string
  text: string
  images: ClarifyImage[]
}

let current: HomeApproveHandoff | null = null

export function setHomeApproveHandoff(next: HomeApproveHandoff) {
  current = {
    runId: next.runId,
    nodeId: next.nodeId,
    text: next.text,
    images: next.images ? next.images.slice() : [],
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

/** Consume only when the parked card matches; otherwise leave the slot intact. */
export function consumeHomeApproveHandoff(runId: string, nodeId: string): HomeApproveHandoff | null {
  const got = current
  if (!got || got.runId !== runId || got.nodeId !== nodeId) return null
  current = null
  return got
}

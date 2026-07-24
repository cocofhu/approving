/** Boot-stage progress for LiveLogPanel empty state (frontend heuristic only). */

export type BootStageId = 'creating' | 'acp_ready' | 'first_event'
export type BootStageState = 'pending' | 'active' | 'done' | 'timeout'

/** Persisted boot dwell state so tab switches (log ↔ sandbox) do not reset timeout. */
export type LiveLogBootSession = {
  confirmedPhase: number | null
  stageEnteredAt: number | null
  timedOut: boolean
}

export const BOOT_STAGE_ORDER: BootStageId[] = ['creating', 'acp_ready', 'first_event']

/** Product wait ceiling (~120s); used for active-stage dwell timeout. */
export const BOOT_STAGE_TIMEOUT_MS = 120_000

export type SandboxPhaseSignal = {
  status?: string | null
  containerStatus?: string | null
} | null | undefined

function norm(s: string | null | undefined): string {
  return (s || '').trim().toLowerCase()
}

/** Container looks ready enough to treat Create as done (ACP wait may still run). */
export function isContainerReady(containerStatus: string | null | undefined): boolean {
  const c = norm(containerStatus)
  return c === 'running' || c === 'up'
}

/**
 * Raw phase index from sandbox + timeline emptiness.
 * 0 = creating, 1 = ACP ready, 2 = waiting first event.
 * Returns null when boot progress should not be shown.
 */
export function deriveBootPhaseIndex(
  nodeStatus: string | undefined,
  sandbox: SandboxPhaseSignal,
  hasTimelineContent: boolean,
): number | null {
  if (nodeStatus !== 'running' || hasTimelineContent) return null

  const sbStatus = norm(sandbox?.status)
  if (sbStatus === 'running') return 2
  if (sbStatus === 'creating' && isContainerReady(sandbox?.containerStatus)) return 1
  // No row, creating without ready container, or uncertain → stay on stage 1.
  return 0
}

/** Monotonic ratchet: never go backwards while still in boot empty state. */
export function ratchetBootPhaseIndex(confirmed: number | null, derived: number | null): number | null {
  if (derived == null) return null
  if (confirmed == null) return derived
  return Math.max(confirmed, derived)
}

export function buildBootStageStates(activeIndex: number, timedOut: boolean): BootStageState[] {
  return BOOT_STAGE_ORDER.map((_, i) => {
    if (i < activeIndex) return 'done'
    if (i === activeIndex) return timedOut ? 'timeout' : 'active'
    return 'pending'
  })
}

export function stageIcon(state: BootStageState): string {
  switch (state) {
    case 'done':
      return 'check'
    case 'active':
      return 'spinner'
    case 'timeout':
      return 'alert'
    default:
      return 'dot'
  }
}

/**
 * Fine-grained run-detail chrome merge (g1.1).
 * Patches status / progress / node runs / artifacts without touching dialogue.
 */
import type { Artifact, Run } from '@/lib/shared/types'
import { artifactFingerprint } from '@/lib/run/reactArtifactPreview'

export function artifactsListFingerprint(arts: Artifact[] | undefined): string {
  if (!arts?.length) return ''
  return arts.map((a) => `${a.name}:${artifactFingerprint(a)}`).join('|')
}

/** Canvas / header fields that may update while a clarify session is streaming. */
export function runChromeFingerprint(run: Run): string {
  const nodeRuns = Object.entries(run.nodeRuns || {})
    .map(([k, v]) => `${k}:${v.status}:${v.durationSec ?? ''}:${v.startedAt || ''}`)
    .sort()
    .join(',')
  const exec = Object.entries(run.nodeExecutions || {})
    .map(([k, list]) => `${k}:${list.length}:${list.map((n) => n.status).join('/')}`)
    .sort()
    .join(',')
  return [
    run.status,
    String(run.progress ?? ''),
    run.startedAt,
    String(run.durationSec ?? ''),
    run.currentNodeLabel || '',
    nodeRuns,
    exec,
    artifactsListFingerprint(run.artifacts),
    run.gate?.nodeId || '',
    run.error || '',
    run.failedReason || '',
    run.failedNode || '',
  ].join('#')
}

export function reactSessionsBusyFingerprint(run: Run): string {
  const sessions = run.reactSessions
  if (!sessions) return ''
  return Object.keys(sessions)
    .sort()
    .map((k) => `${k}:${sessions[k]?.busy ? '1' : '0'}:${sessions[k]?.waiting ?? 0}`)
    .join('|')
}

/**
 * Merge REST snapshot chrome onto the live run.
 * Keeps reactSessions / clarify turns so a busy dialogue is not overwritten (g1.1).
 */
export function mergeRunChromeFields(current: Run, snapshot: Run): Run {
  const nextArts = snapshot.artifacts ?? current.artifacts
  const artifacts =
    artifactsListFingerprint(current.artifacts) === artifactsListFingerprint(nextArts)
      ? current.artifacts
      : nextArts
  const nodeRuns = snapshot.nodeRuns ?? current.nodeRuns
  const nodeExecutions = snapshot.nodeExecutions ?? current.nodeExecutions
  return {
    ...current,
    status: snapshot.status,
    progress: snapshot.progress,
    startedAt: snapshot.startedAt,
    durationSec: snapshot.durationSec,
    currentNodeLabel: snapshot.currentNodeLabel,
    nodeRuns,
    nodeExecutions,
    artifacts,
    gate: snapshot.gate,
    trace: snapshot.trace ?? current.trace,
    git: snapshot.git !== undefined ? snapshot.git : current.git,
    error: snapshot.error,
    failedReason: snapshot.failedReason,
    failedNode: snapshot.failedNode,
    priority: snapshot.priority ?? current.priority,
    // reactSessions, clarify, clarifyByNode stay on `current` via spread.
  }
}

export function runSnapshotUnchanged(current: Run, snapshot: Run): boolean {
  return (
    current.id === snapshot.id &&
    runChromeFingerprint(current) === runChromeFingerprint(snapshot) &&
    reactSessionsBusyFingerprint(current) === reactSessionsBusyFingerprint(snapshot)
  )
}

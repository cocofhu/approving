import type { Artifact } from '@/lib/shared/types'

/** Snapshot value that may appear in artifact.workflowName from the API. */
const UNNAMED_WORKFLOW_SNAPSHOT = '未命名工作流'
/** @deprecated Use UNNAMED_WORKFLOW_SNAPSHOT in fixtures; UI title comes from i18n. */
export const UNNAMED_WORKFLOW = UNNAMED_WORKFLOW_SNAPSHOT
export const UNNAMED_GROUP_KEY = '__unnamed__'

export interface ArtifactGroup {
  key: string
  workflowId: string | null
  title: string
  count: number
  artifacts: Artifact[]
  isUnnamed: boolean
}

export interface RunSection {
  runId: string
  runTitle?: string
  items: Artifact[]
  latestAt: number
}

export type WorkflowMap = Map<string, { name: string }> | Record<string, { name: string }>

export function isUnnamed(a: Artifact): boolean {
  return !a.workflowId
}

export function groupKey(a: Artifact): string {
  return isUnnamed(a) ? UNNAMED_GROUP_KEY : a.workflowId!
}

function sortByCreatedAtDesc(artifacts: Artifact[]): Artifact[] {
  return [...artifacts].sort((x, y) => new Date(y.createdAt).getTime() - new Date(x.createdAt).getTime())
}

function resolveGroupTitle(
  workflowId: string | null,
  snapshotName: string,
  workflows?: WorkflowMap,
): string {
  if (!workflowId) return ''
  const wf =
    workflows instanceof Map ? workflows.get(workflowId) : workflows?.[workflowId]
  if (wf?.name) return wf.name
  return snapshotName || ''
}

export function buildGroups(artifacts: Artifact[], workflows?: WorkflowMap): ArtifactGroup[] {
  const map = new Map<
    string,
    { key: string; workflowId: string | null; artifacts: Artifact[]; latest: Artifact }
  >()

  for (const a of artifacts) {
    const key = groupKey(a)
    if (!map.has(key)) {
      map.set(key, {
        key,
        workflowId: key === UNNAMED_GROUP_KEY ? null : a.workflowId ?? null,
        artifacts: [],
        latest: a,
      })
    }
    const g = map.get(key)!
    g.artifacts.push(a)
    if (new Date(a.createdAt) > new Date(g.latest.createdAt)) g.latest = a
  }

  const groups: ArtifactGroup[] = [...map.values()].map((g) => ({
    key: g.key,
    workflowId: g.workflowId,
    title:
      g.key === UNNAMED_GROUP_KEY
        ? ''
        : resolveGroupTitle(g.workflowId, g.latest.workflowName, workflows),
    count: g.artifacts.length,
    artifacts: sortByCreatedAtDesc(g.artifacts),
    isUnnamed: g.key === UNNAMED_GROUP_KEY,
  }))

  return groups.sort((a, b) => {
    if (a.isUnnamed) return 1
    if (b.isUnnamed) return -1
    return b.count - a.count
  })
}

export function visibleGroups(allGroups: ArtifactGroup[], wfParam: string): ArtifactGroup[] {
  if (wfParam) return allGroups.filter((g) => g.workflowId === wfParam)
  return allGroups
}

export function resolveDefaultGroup(
  groups: ArtifactGroup[],
  options: { highlightUnnamed?: boolean; wfParam?: string } = {},
): ArtifactGroup | null {
  if (!groups.length) return null
  const { highlightUnnamed = false, wfParam = '' } = options
  if (highlightUnnamed) return groups.find((g) => g.isUnnamed) ?? groups[0]
  if (wfParam) return groups.find((g) => g.workflowId === wfParam) ?? groups[0]
  const named = groups.filter((g) => !g.isUnnamed)
  const pool = named.length ? named : groups
  return pool.reduce((best, g) => (g.count > best.count ? g : best), pool[0])
}

export function countUnnamedArtifacts(artifacts: Artifact[]): number {
  return artifacts.filter(isUnnamed).length
}

/** Short run id for section titles: run-abc123 → #abc123 (matches RunListView strip + #). */
export function runIdShort(id: string): string {
  return '#' + id.replace(/^run-/, '')
}

export function runSectionTitle(runTitle: string | undefined, runId: string): string {
  return runTitle?.trim() ? runTitle : runIdShort(runId)
}

export function groupByRun(artifacts: Artifact[]): RunSection[] {
  const map = new Map<string, RunSection>()
  for (const a of artifacts) {
    if (!map.has(a.runId)) {
      map.set(a.runId, { runId: a.runId, runTitle: a.runTitle, items: [], latestAt: 0 })
    }
    map.get(a.runId)!.items.push(a)
  }
  for (const sec of map.values()) {
    sec.items = sortByCreatedAtDesc(sec.items)
    sec.latestAt = Math.max(...sec.items.map((i) => new Date(i.createdAt).getTime()))
  }
  return [...map.values()].sort((a, b) => b.latestAt - a.latestAt)
}

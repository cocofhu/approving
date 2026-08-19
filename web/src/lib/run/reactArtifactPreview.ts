import type { Artifact, NodeRun, Run, WFNode } from '@/lib/shared/types'
import { productArtifactName } from '@/lib/run/productNodeArtifacts'

export const REACT_STAGE_TAB_GRID = 'grid'
export const REACT_STAGE_TAB_NOVNC = 'novnc'
export const PREVIEW_TAB_PREFIX = 'preview:'

export type ReactStageRemoteKind = 'sandbox' | 'app' | 'public' | 'off'

/** Own-node artifacts can be annotated; foreign / unnamed-own stay view-only. */
export function isOwnNodeArtifact(artifact: Pick<Artifact, 'nodeId'> | null | undefined, nodeId?: string | null): boolean {
  const own = String(nodeId || '').trim()
  if (!own) return true
  const artNode = String(artifact?.nodeId || '').trim()
  if (!artNode) return true
  return artNode === own
}

export function canAnnotateStageArtifact(
  annotatable: boolean,
  artifact: (Pick<Artifact, 'nodeId'> & Partial<Pick<Artifact, 'id'>>) | null | undefined,
  nodeId?: string | null,
): boolean {
  return !!annotatable && isOwnNodeArtifact(artifact, nodeId) && !isHistoricalStageArtifact(artifact)
}

const HISTORICAL_PAGE_ID = /^historical-page:([^:]+):(\d+)$/

export function historicalStageArtifactId(nodeId: string, iteration: number): string {
  return `historical-page:${nodeId}:${iteration}`
}

export function parseHistoricalStageArtifact(
  artifact: Partial<Pick<Artifact, 'id'>> | null | undefined,
): { nodeId: string; iteration: number } | null {
  const m = HISTORICAL_PAGE_ID.exec(String(artifact?.id || ''))
  if (!m) return null
  return { nodeId: m[1], iteration: Number(m[2]) }
}

export function isHistoricalStageArtifact(artifact: Partial<Pick<Artifact, 'id'>> | null | undefined): boolean {
  return parseHistoricalStageArtifact(artifact) != null
}

export function historicalStageArtifactName(baseName: string, iteration: number): string {
  return `${baseName}#iter-${iteration}`
}

export function listVisualPageVersions(run: Run | null | undefined, nodeId: string | null | undefined): NodeRun[] {
  if (!run || !nodeId) return []
  return (run.nodeExecutions?.[nodeId] || [])
    .filter(
      (nodeRun) =>
        (nodeRun.status === 'completed' || nodeRun.status === 'waiting_human') &&
        typeof nodeRun.outputs?.page === 'string' &&
        nodeRun.outputs.page.trim().length > 0,
    )
    .sort((a, b) => (a.iteration ?? 0) - (b.iteration ?? 0))
}

/** Visual review: keep live page.html, plus readonly snapshots of earlier outputs.page. */
export function expandStageArtifacts(
  artifacts: Artifact[],
  run?: Run | null,
  node?: WFNode | null,
): Artifact[] {
  if (!run || !node || node.type !== 'visual') return artifacts
  const versions = listVisualPageVersions(run, node.id)
  if (versions.length < 2) return artifacts
  const latestIter = versions[versions.length - 1]?.iteration
  const baseName = productArtifactName('visual') || 'page.html'
  const liveIdx = artifacts.findIndex((a) => a.name === baseName && isOwnNodeArtifact(a, node.id))
  const live = liveIdx >= 0 ? artifacts[liveIdx] : undefined
  const historical: Artifact[] = versions
    .filter((v) => v.iteration !== latestIter)
    .map((v) => {
      const html = String(v.outputs?.page || '')
      const iteration = v.iteration ?? 0
      return {
        id: historicalStageArtifactId(node.id, iteration),
        name: historicalStageArtifactName(baseName, iteration),
        kind: 'html',
        nodeId: node.id,
        runId: live?.runId || run.id || '',
        workflowName: live?.workflowName || '',
        sizeBytes: html.length,
        createdAt: live?.createdAt || '',
        content: html,
        revision: Math.max(1, iteration),
      }
    })
  if (!historical.length) return artifacts
  if (liveIdx < 0) return [...historical, ...artifacts]
  return [...artifacts.slice(0, liveIdx), ...historical, artifacts[liveIdx], ...artifacts.slice(liveIdx + 1)]
}

export function resolveStageRemoteKind(opts: {
  remoteKind?: ReactStageRemoteKind | null
  runId?: string | null
  nodeId?: string | null
  inlineContent?: boolean
}): ReactStageRemoteKind {
  if (opts.remoteKind) return opts.remoteKind
  if (opts.inlineContent) return 'off'
  if (opts.runId && opts.nodeId) return 'sandbox'
  return 'off'
}

export type ReactStageTab = typeof REACT_STAGE_TAB_GRID | string

export function previewTabId(name: string): string {
  return PREVIEW_TAB_PREFIX + name
}

export function previewTabName(tabId: string): string | null {
  if (!tabId.startsWith(PREVIEW_TAB_PREFIX)) return null
  const n = tabId.slice(PREVIEW_TAB_PREFIX.length)
  return n || null
}

/** Append a preview tab; clicking an already-open artifact only focuses it. */
export function openStagePreviewTab(openNames: string[], name: string): string[] {
  const n = String(name || '').trim()
  if (!n) return openNames
  if (openNames.includes(n)) return openNames
  return [...openNames, n]
}

export function closeStagePreviewTab(openNames: string[], name: string): string[] {
  return openNames.filter((n) => n !== name)
}

/** After closing a tab, stay on the current one, else neighbor, else noVNC/grid. */
export function nextTabAfterClose(
  openNames: string[],
  closed: string,
  currentTab: string,
  novncOpen = false,
): string {
  const remaining = closeStagePreviewTab(openNames, closed)
  const currentName = previewTabName(currentTab)
  if (currentName !== closed) {
    return currentTab
  }
  if (!remaining.length) {
    return novncOpen ? REACT_STAGE_TAB_NOVNC : REACT_STAGE_TAB_GRID
  }
  const i = openNames.indexOf(closed)
  const pick = remaining[Math.min(Math.max(i, 0), remaining.length - 1)] ?? remaining[remaining.length - 1]
  return previewTabId(pick)
}

export type ArtifactKindKey = Artifact['kind']

const KIND_I18N: Record<string, string> = {
  html: 'pages.reactArtifactStage.kindHtml',
  json: 'pages.reactArtifactStage.kindJson',
  markdown: 'pages.reactArtifactStage.kindMarkdown',
  yaml: 'pages.reactArtifactStage.kindYaml',
  image: 'pages.reactArtifactStage.kindImage',
  text: 'pages.reactArtifactStage.kindText',
}

export function artifactKindLabelKey(kind: string | undefined): string {
  return KIND_I18N[kind || ''] || 'pages.reactArtifactStage.kindFile'
}

export function artifactRevision(a: Pick<Artifact, 'revision'> | null | undefined): number {
  const n = a?.revision
  if (typeof n === 'number' && Number.isFinite(n) && n >= 1) return Math.floor(n)
  return 1
}

export function findArtifactByName(artifacts: Artifact[], name: string | null | undefined): Artifact | null {
  const n = String(name || '').trim()
  if (!n) return null
  return artifacts.find((a) => a.name === n) || null
}

export function isReactGraphNode(run: Run | null | undefined, nodeId: string | null | undefined): boolean {
  if (!run?.nodes?.length || !nodeId) return false
  return run.nodes.some((n: WFNode) => n.id === nodeId && n.type === 'react')
}

export function inboxStageRemoteKind(opts: {
  appPreview: boolean
  run?: Run | null
  nodeId?: string | null
}): ReactStageRemoteKind {
  if (opts.appPreview) return 'app'
  if (isReactGraphNode(opts.run, opts.nodeId)) return 'sandbox'
  return 'off'
}

/** Copy artifacts + previewArtifact without replacing chat turns / sessions. */
export function applyPreviewArtifactFromRun(current: Run, incoming: Run): Run {
  const next: Run = { ...current, artifacts: incoming.artifacts ?? current.artifacts }
  if (incoming.clarify) {
    next.clarify = current.clarify
      ? { ...current.clarify, previewArtifact: incoming.clarify.previewArtifact }
      : incoming.clarify
  }
  if (incoming.clarifyByNode) {
    const byNode = { ...(current.clarifyByNode || {}) }
    for (const [id, conv] of Object.entries(incoming.clarifyByNode)) {
      const cur = byNode[id]
      byNode[id] = cur ? { ...cur, previewArtifact: conv.previewArtifact } : conv
    }
    next.clarifyByNode = byNode
  }
  return next
}

export function applyPreviewArtifactName(current: Run, nodeId: string, name: string): Run {
  const n = String(name || '').trim()
  if (!n || !nodeId) return current
  const byNode = { ...(current.clarifyByNode || {}) }
  const fromClarify = current.clarify?.nodeId === nodeId ? current.clarify : undefined
  const cur = byNode[nodeId] || fromClarify
  if (cur) byNode[nodeId] = { ...cur, previewArtifact: n }
  const clarify = fromClarify ? { ...fromClarify, previewArtifact: n } : current.clarify
  return { ...current, clarifyByNode: byNode, clarify }
}

/** Open/focus the pinned tab when the pin changes or the named artifact first appears. */
export function shouldActivatePinnedPreview(
  pin: string,
  names: string[],
  prevPin?: string,
  prevNames?: string[],
): boolean {
  const n = String(pin || '').trim()
  if (!n || !names.includes(n)) return false
  if (prevPin === undefined || prevNames === undefined) return true
  return n !== prevPin || !prevNames.includes(n)
}

export function artifactFingerprint(a: Artifact | null | undefined): string {
  if (!a) return ''
  return `${a.id}:${a.updatedAt || ''}:${a.revision ?? ''}:${a.sizeBytes}:${a.content?.length ?? ''}`
}

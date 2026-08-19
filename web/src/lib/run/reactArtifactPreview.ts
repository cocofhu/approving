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

export function visualProductPageName(): string {
  return productArtifactName('visual') || 'page.html'
}

/** Physical copy written by finalizeVisual: `{nodeId}.page.html`. */
export function visualNodePageName(nodeId: string): string {
  return `${String(nodeId || '').trim()}.page.html`
}

export function parseVisualNodePageName(name: string | null | undefined): string | null {
  const n = String(name || '')
  const suffix = '.page.html'
  if (!n.endsWith(suffix)) return null
  const nodeId = n.slice(0, -suffix.length)
  if (!nodeId || n === visualProductPageName()) return null
  return nodeId
}

export function visualPageOwnerNodeId(artifact: Pick<Artifact, 'name' | 'nodeId'> | null | undefined): string {
  if (!artifact) return ''
  const fromName = parseVisualNodePageName(artifact.name)
  if (fromName) return fromName
  if (artifact.name === visualProductPageName()) return String(artifact.nodeId || '').trim()
  return ''
}

/** Hide `{nodeId}.page.html` when page.html from the same visual node is also in the grid. */
export function isSamePreviewVisualCopy(
  artifact: Pick<Artifact, 'name' | 'nodeId'>,
  artifacts: Pick<Artifact, 'name' | 'nodeId'>[],
): boolean {
  const owner = parseVisualNodePageName(artifact.name)
  if (!owner) return false
  const page = artifacts.find((a) => a.name === visualProductPageName())
  if (!page) return false
  return String(page.nodeId || '').trim() === owner
}

export function listVisualNodeRuns(run: Run | null | undefined, nodeId: string | null | undefined): NodeRun[] {
  if (!run || !nodeId) return []
  return (run.nodeExecutions?.[nodeId] || [])
    .filter((nodeRun) => nodeRun.status === 'completed' || nodeRun.status === 'waiting_human')
    .sort((a, b) => (a.iteration ?? 0) - (b.iteration ?? 0))
}

export function listVisualPageVersions(run: Run | null | undefined, nodeId: string | null | undefined): NodeRun[] {
  return listVisualNodeRuns(run, nodeId).filter(
    (nodeRun) => typeof nodeRun.outputs?.page === 'string' && nodeRun.outputs.page.trim().length > 0,
  )
}

export type VisualPageVersionChoice = {
  index: number
  iteration: number
  latest: boolean
  available: boolean
  html: string
}

/** User-visible v1..vN mapped to visual nodeExecutions; latest follows live artifact bytes. */
export function listVisualPageVersionChoices(
  run: Run | null | undefined,
  artifact: Pick<Artifact, 'name' | 'nodeId' | 'kind'> | null | undefined,
): VisualPageVersionChoice[] {
  if (!run || !artifact) return []
  if (artifact.kind && artifact.kind !== 'html') return []
  const nodeId = visualPageOwnerNodeId(artifact)
  if (!nodeId) return []
  const runs = listVisualNodeRuns(run, nodeId)
  if (!runs.length) return []
  return runs.map((nodeRun, i) => {
    const latest = i === runs.length - 1
    const html = String(nodeRun.outputs?.page || '')
    const available = latest || html.trim().length > 0
    return {
      index: i + 1,
      iteration: nodeRun.iteration ?? i + 1,
      latest,
      available,
      html: latest ? '' : html,
    }
  })
}

export function resolveVisualPagePreviewArtifact(
  artifact: Artifact,
  choice: VisualPageVersionChoice | null | undefined,
): Artifact {
  if (!choice || choice.latest || !choice.available) return artifact
  const nodeId = visualPageOwnerNodeId(artifact)
  return {
    ...artifact,
    id: historicalStageArtifactId(nodeId || artifact.nodeId, choice.iteration),
    content: choice.html,
    sizeBytes: choice.html.length,
  }
}

/**
 * Grid display list: never flatten page.html#iter-N into extra cards.
 * Hide same-preview visual_{nodeId}.page.html when page.html is present.
 */
export function expandStageArtifacts(
  artifacts: Artifact[],
  _run?: Run | null,
  _node?: WFNode | null,
): Artifact[] {
  const list = artifacts.filter((a) => !isHistoricalStageArtifact(a))
  const page = list.find((a) => a.name === visualProductPageName())
  if (!page) return list
  const owner = String(page.nodeId || '').trim()
  if (!owner) return list
  const alias = visualNodePageName(owner)
  return list.filter((a) => a.name !== alias)
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

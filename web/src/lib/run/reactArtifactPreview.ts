import type { Artifact, Run, WFNode } from '@/lib/shared/types'

export const REACT_STAGE_TAB_GRID = 'grid'
export const REACT_STAGE_TAB_NOVNC = 'novnc'
export const PREVIEW_TAB_PREFIX = 'preview:'

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

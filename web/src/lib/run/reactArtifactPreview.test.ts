import { describe, expect, it } from 'vitest'
import type { Artifact, Run } from '@/lib/shared/types'
import {
  REACT_STAGE_TAB_GRID,
  REACT_STAGE_TAB_NOVNC,
  applyPreviewArtifactFromRun,
  applyPreviewArtifactName,
  artifactFingerprint,
  canAnnotateStageArtifact,
  shouldActivatePinnedPreview,
  artifactKindLabelKey,
  artifactRevision,
  closeStagePreviewTab,
  findArtifactByName,
  isOwnNodeArtifact,
  isReactGraphNode,
  nextTabAfterClose,
  openStagePreviewTab,
  previewTabId,
  resolveStageRemoteKind,
} from './reactArtifactPreview'

function art(partial: Partial<Artifact> & Pick<Artifact, 'id' | 'name'>): Artifact {
  return {
    kind: 'html',
    nodeId: 'react',
    runId: 'r1',
    workflowName: 'wf',
    sizeBytes: 10,
    createdAt: '2026-08-01T00:00:00Z',
    ...partial,
  }
}

describe('reactArtifactPreview helpers', () => {
  it('treats missing revision as v1 and increments from stored values', () => {
    expect(artifactRevision(undefined)).toBe(1)
    expect(artifactRevision({ revision: 0 })).toBe(1)
    expect(artifactRevision({ revision: 3 })).toBe(3)
  })

  it('finds artifacts by name and fingerprints identity+version', () => {
    const a = art({ id: 'a1', name: 'page.html', revision: 2, updatedAt: 't2', sizeBytes: 8 })
    expect(findArtifactByName([a], 'page.html')?.id).toBe('a1')
    expect(findArtifactByName([a], 'missing')).toBeNull()
    expect(artifactFingerprint(a)).toBe('a1:t2:2:8:')
  })

  it('maps kind to i18n keys', () => {
    expect(artifactKindLabelKey('html')).toContain('kindHtml')
    expect(artifactKindLabelKey('unknown')).toContain('kindFile')
  })

  it('detects react graph nodes', () => {
    const run = {
      nodes: [{ id: 'c1', type: 'react', label: '澄清', position: { x: 0, y: 0 }, config: {} }],
    } as Run
    expect(isReactGraphNode(run, 'c1')).toBe(true)
    expect(isReactGraphNode(run, 'other')).toBe(false)
  })

  it('treats foreign-node artifacts as read-only unless nodeId is empty', () => {
    expect(isOwnNodeArtifact({ nodeId: 'research' }, '')).toBe(true)
    expect(isOwnNodeArtifact({ nodeId: 'research' }, 'research')).toBe(true)
    expect(isOwnNodeArtifact({ nodeId: 'plan' }, 'research')).toBe(false)
    expect(isOwnNodeArtifact({ nodeId: '' }, 'research')).toBe(true)
    expect(canAnnotateStageArtifact(true, { nodeId: 'plan' }, 'research')).toBe(false)
    expect(canAnnotateStageArtifact(true, { nodeId: 'research' }, 'research')).toBe(true)
    expect(canAnnotateStageArtifact(false, { nodeId: 'research' }, 'research')).toBe(false)
  })

  it('resolves remoteKind with explicit override over sandbox default', () => {
    expect(resolveStageRemoteKind({ runId: 'r', nodeId: 'n' })).toBe('sandbox')
    expect(resolveStageRemoteKind({ runId: 'r', nodeId: 'n', inlineContent: true })).toBe('off')
    expect(resolveStageRemoteKind({ runId: 'r', nodeId: 'n', remoteKind: 'app' })).toBe('app')
    expect(resolveStageRemoteKind({ inlineContent: true, remoteKind: 'public' })).toBe('public')
    expect(resolveStageRemoteKind({})).toBe('off')
  })

  it('opens preview tabs without replacing already-open ones', () => {
    expect(openStagePreviewTab([], 'a.html')).toEqual(['a.html'])
    expect(openStagePreviewTab(['a.html'], 'b.md')).toEqual(['a.html', 'b.md'])
    expect(openStagePreviewTab(['a.html', 'b.md'], 'a.html')).toEqual(['a.html', 'b.md'])
    expect(closeStagePreviewTab(['a.html', 'b.md'], 'a.html')).toEqual(['b.md'])
    expect(nextTabAfterClose(['a.html', 'b.md'], 'b.md', previewTabId('b.md'))).toBe(previewTabId('a.html'))
    expect(nextTabAfterClose(['a.html'], 'a.html', previewTabId('a.html'))).toBe(REACT_STAGE_TAB_GRID)
    expect(nextTabAfterClose(['a.html', 'b.md'], 'a.html', previewTabId('b.md'))).toBe(previewTabId('b.md'))
    expect(nextTabAfterClose(['a.html'], 'a.html', previewTabId('a.html'), true)).toBe(REACT_STAGE_TAB_NOVNC)
    expect(nextTabAfterClose(['a.html'], 'a.html', REACT_STAGE_TAB_NOVNC)).toBe(REACT_STAGE_TAB_NOVNC)
  })

  it('patches previewArtifact without replacing turns', () => {
    const current = {
      id: 'r1',
      artifacts: [art({ id: 'a1', name: 'old.html' })],
      clarify: { nodeId: 'c1', turns: [{ role: 'agent', text: 'hi', at: 't' }], done: false },
      clarifyByNode: {
        c1: { nodeId: 'c1', turns: [{ role: 'agent', text: 'hi', at: 't' }], done: false },
      },
    } as unknown as Run
    const named = applyPreviewArtifactName(current, 'c1', 'page.html')
    expect(named.clarify?.previewArtifact).toBe('page.html')
    expect(named.clarifyByNode?.c1.turns).toHaveLength(1)

    const onlyTop = {
      id: 'r2',
      artifacts: [],
      clarify: { nodeId: 'c2', turns: [{ role: 'agent', text: 'keep', at: 't' }], done: false },
    } as unknown as Run
    const filled = applyPreviewArtifactName(onlyTop, 'c2', 'note.md')
    expect(filled.clarifyByNode?.c2.previewArtifact).toBe('note.md')
    expect(filled.clarifyByNode?.c2.turns[0].text).toBe('keep')
    const incoming = {
      artifacts: [art({ id: 'a2', name: 'page.html', revision: 4 })],
      clarifyByNode: { c1: { nodeId: 'c1', turns: [], done: false, previewArtifact: 'page.html' } },
    } as unknown as Run
    const merged = applyPreviewArtifactFromRun(named, incoming)
    expect(merged.artifacts[0].name).toBe('page.html')
    expect(merged.clarifyByNode?.c1.previewArtifact).toBe('page.html')
    expect(merged.clarifyByNode?.c1.turns[0].text).toBe('hi')
  })

  it('activates a pin when it changes or the named artifact first appears', () => {
    expect(shouldActivatePinnedPreview('page.html', ['note.md'])).toBe(false)
    expect(shouldActivatePinnedPreview('page.html', ['page.html'])).toBe(true)
    expect(shouldActivatePinnedPreview('page.html', ['page.html'], '', [])).toBe(true)
    expect(shouldActivatePinnedPreview('page.html', ['page.html'], 'page.html', ['note.md'])).toBe(true)
    expect(shouldActivatePinnedPreview('b.md', ['a.html', 'b.md'], 'a.html', ['a.html', 'b.md'])).toBe(true)
    expect(shouldActivatePinnedPreview('page.html', ['page.html', 'extra.md'], 'page.html', ['page.html'])).toBe(
      false,
    )
  })
})

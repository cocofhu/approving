import { describe, expect, it } from 'vitest'
import type { Artifact, Run } from '@/lib/shared/types'
import {
  REACT_STAGE_TAB_GRID,
  REACT_STAGE_TAB_NOVNC,
  applyPreviewArtifactFromRun,
  applyPreviewArtifactName,
  artifactFingerprint,
  canAnnotateStageArtifact,
  expandStageArtifacts,
  filterStageGridArtifacts,
  isKnownStageGridArtifact,
  historicalStageArtifactId,
  inboxStageRemoteKind,
  isSamePreviewVisualCopy,
  listVisualPageVersionChoices,
  pageHistoryEntries,
  resolveVisualPagePreviewArtifact,
  visualNodePageName,
  isHistoricalStageArtifact,
  shouldActivatePinnedPreview,
  artifactKindLabelKey,
  artifactRevision,
  closeStagePreviewTab,
  findArtifactByName,
  isOwnNodeArtifact,
  isReactGraphNode,
  latestOwnNodeHtmlName,
  nextTabAfterClose,
  openStagePreviewTab,
  previewTabId,
  resolveEffectivePreviewPin,
  resolveStageRemoteKind,
  stageGridArtifactsWithPin,
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
    expect(canAnnotateStageArtifact(true, { id: historicalStageArtifactId('visual', 1), nodeId: 'visual' }, 'visual')).toBe(false)
    expect(isHistoricalStageArtifact({ id: historicalStageArtifactId('visual', 1) })).toBe(true)
  })

  it('does not flatten historical visual snapshots into extra grid cards', () => {
    const live = art({ id: 'live', name: 'page.html', nodeId: 'visual_1', content: '<p>new</p>' })
    const run = {
      id: 'r1',
      nodes: [{ id: 'visual_1', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} }],
      nodeExecutions: {
        visual_1: [
          { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>old</p>' } },
          { nodeId: 'visual_1', iteration: 2, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
        ],
      },
    } as unknown as Run
    const node = run.nodes![0]
    expect(expandStageArtifacts([live], run, node).map((a) => a.name)).toEqual(['page.html'])
    expect(expandStageArtifacts([live], run, { ...node, type: 'research' }).map((a) => a.name)).toEqual(['page.html'])
  })

  it('hides the same-preview visual node page copy when page.html is present', () => {
    const page = art({ id: 'p', name: 'page.html', nodeId: 'visual_bqc5', content: '<p>same</p>' })
    const alias = art({ id: 'a', name: visualNodePageName('visual_bqc5'), nodeId: 'visual_bqc5', content: '<p>same</p>' })
    const other = art({ id: 'o', name: visualNodePageName('visual_other'), nodeId: 'visual_other', content: '<p>diff</p>' })
    const json = art({ id: 'j', name: 'node_complete.json', kind: 'json', nodeId: 'visual_bqc5' })
    const collapsed = expandStageArtifacts([page, alias, other, json])
    expect(collapsed.map((a) => a.name)).toEqual(['page.html', visualNodePageName('visual_other'), 'node_complete.json'])
    expect(isSamePreviewVisualCopy(alias, [page, alias])).toBe(true)
    expect(isSamePreviewVisualCopy(other, [page, alias, other])).toBe(false)
  })

  it('keeps a visual_*.page.html card when page.html is absent', () => {
    const alias = art({ id: 'a', name: visualNodePageName('visual_1'), nodeId: 'visual_1' })
    expect(expandStageArtifacts([alias]).map((a) => a.name)).toEqual([visualNodePageName('visual_1')])
  })

  it('maps v-axis to outputs.page and does not substitute latest html for missing snapshots', () => {
    const live = art({ id: 'live', name: 'page.html', nodeId: 'visual_1', content: '<p>new</p>' })
    const run = {
      nodeExecutions: {
        visual_1: [
          { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: {} },
          { nodeId: 'visual_1', iteration: 2, status: 'completed', outputs: { page: '<p>mid</p>' } },
          { nodeId: 'visual_1', iteration: 3, status: 'waiting_human', outputs: { page: '<p>new</p>' } },
        ],
      },
    } as unknown as Run
    const choices = listVisualPageVersionChoices(run, live)
    expect(choices).toHaveLength(3)
    expect(choices[0]).toMatchObject({ index: 1, latest: false, available: false, html: '' })
    expect(choices[1]).toMatchObject({ index: 2, latest: false, available: true, html: '<p>mid</p>' })
    expect(choices[2]).toMatchObject({ index: 3, latest: true, available: true, html: '' })
    const v2 = resolveVisualPagePreviewArtifact(live, choices[1])
    expect(v2.content).toBe('<p>mid</p>')
    expect(v2.id).toBe(historicalStageArtifactId('visual_1', 2))
    const missing = resolveVisualPagePreviewArtifact(live, choices[0])
    expect(missing).toBe(live)
    const latest = resolveVisualPagePreviewArtifact(live, choices[2])
    expect(latest).toBe(live)
    expect(listVisualPageVersionChoices(run, art({ id: 'j', name: 'node_complete.json', kind: 'json', nodeId: 'visual_1' }))).toEqual([])
  })

  it('expands page_history from a single visual execution into version choices', () => {
    const live = art({ id: 'live', name: 'page.html', nodeId: 'visual_1', content: '<p>v3</p>' })
    const run = {
      nodeExecutions: {
        visual_1: [
          {
            nodeId: 'visual_1',
            iteration: 1,
            status: 'waiting_human',
            outputs: { page: '<p>v3</p>', page_history: ['<p>v1</p>', '<p>v2</p>'] },
          },
        ],
      },
    } as unknown as Run
    expect(pageHistoryEntries({ page_history: ['<p>v1</p>', 1, ''] })).toEqual(['<p>v1</p>'])
    const choices = listVisualPageVersionChoices(run, live)
    expect(choices).toHaveLength(3)
    expect(choices[0]).toMatchObject({ index: 1, latest: false, available: true, html: '<p>v1</p>' })
    expect(choices[1]).toMatchObject({ index: 2, latest: false, available: true, html: '<p>v2</p>' })
    expect(choices[2]).toMatchObject({ index: 3, latest: true, available: true, html: '' })
    expect(resolveVisualPagePreviewArtifact(live, choices[0]).content).toBe('<p>v1</p>')
    expect(resolveVisualPagePreviewArtifact(live, choices[2])).toBe(live)
  })

  it('keeps prior visual iterations and appends page_history on the latest run', () => {
    const live = art({ id: 'live', name: 'page.html', nodeId: 'visual_1', content: '<p>new</p>' })
    const run = {
      nodeExecutions: {
        visual_1: [
          { nodeId: 'visual_1', iteration: 1, status: 'completed', outputs: { page: '<p>old</p>' } },
          {
            nodeId: 'visual_1',
            iteration: 2,
            status: 'waiting_human',
            outputs: { page: '<p>new</p>', page_history: ['<p>mid</p>'] },
          },
        ],
      },
    } as unknown as Run
    const choices = listVisualPageVersionChoices(run, live)
    expect(choices.map((c) => ({ index: c.index, latest: c.latest, html: c.html }))).toEqual([
      { index: 1, latest: false, html: '<p>old</p>' },
      { index: 2, latest: false, html: '<p>mid</p>' },
      { index: 3, latest: true, html: '' },
    ])
  })

  it('uses sandbox remote only for ReAct inbox nodes', () => {
    const run = {
      nodes: [
        { id: 'c1', type: 'react', label: '澄清', position: { x: 0, y: 0 }, config: {} },
        { id: 'visual', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} },
      ],
    } as unknown as Run
    expect(inboxStageRemoteKind({ appPreview: true, run, nodeId: 'preview' })).toBe('app')
    expect(inboxStageRemoteKind({ appPreview: false, run, nodeId: 'c1' })).toBe('sandbox')
    expect(inboxStageRemoteKind({ appPreview: false, run, nodeId: 'visual' })).toBe('off')
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
    expect(shouldActivatePinnedPreview('page.html', ['page.html'], '', [], true)).toBe(false)
    expect(shouldActivatePinnedPreview('page.html', ['page.html'], undefined, undefined, true)).toBe(false)
  })

  it('prefers an on-stage pin, then visual page.html, then newest own-node HTML', () => {
    const pin = art({ id: 'pin', name: 'brief.md', kind: 'markdown', nodeId: 'react' })
    const live = art({ id: 'live', name: 'page.html', kind: 'html', nodeId: 'visual_bqc5' })
    const copy = art({ id: 'copy', name: 'visual_bqc5.page.html', kind: 'html', nodeId: 'visual_bqc5' })
    const hist = art({ id: 'hist', name: 'page.html#iter-1', kind: 'html', nodeId: 'visual_bqc5' })
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: 'brief.md',
        artifacts: [live, pin],
        nodeType: 'visual',
        nodeId: 'visual_bqc5',
      }),
    ).toBe('brief.md')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: 'missing.html',
        artifacts: [live],
        nodeType: 'visual',
        nodeId: 'visual_bqc5',
      }),
    ).toBe('')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: '',
        artifacts: [copy, hist, live],
        nodeType: 'visual',
        nodeId: 'visual_bqc5',
      }),
    ).toBe('page.html')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: '',
        artifacts: [copy],
        nodeType: 'visual',
        nodeId: 'visual_bqc5',
      }),
    ).toBe('')
  })

  it('picks the newest own-node HTML for unpinned react and ignores upstream page.html', () => {
    const upstream = art({
      id: 'up',
      name: 'page.html',
      kind: 'html',
      nodeId: 'visual_bqc5',
      updatedAt: '2026-08-19T20:00:00Z',
      revision: 9,
    })
    const older = art({
      id: 'old',
      name: 'a.html',
      kind: 'html',
      nodeId: 'react_ymx0',
      updatedAt: '2026-08-19T10:00:00Z',
      revision: 2,
    })
    const newer = art({
      id: 'new',
      name: 'brand-row-preview.html',
      kind: 'html',
      nodeId: 'react_ymx0',
      updatedAt: '2026-08-19T12:00:00Z',
      revision: 1,
    })
    const json = art({ id: 'j', name: 'research.json', kind: 'json', nodeId: 'react_ymx0' })
    expect(latestOwnNodeHtmlName([upstream, older, newer, json], 'react_ymx0')).toBe('brand-row-preview.html')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: '',
        artifacts: [upstream, older, newer, json],
        nodeType: 'react',
        nodeId: 'react_ymx0',
      }),
    ).toBe('brand-row-preview.html')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: 'research.json',
        artifacts: [upstream, older, newer, json],
        nodeType: 'react',
        nodeId: 'react_ymx0',
      }),
    ).toBe('research.json')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: '',
        artifacts: [json],
        nodeType: 'react',
        nodeId: 'react_ymx0',
      }),
    ).toBe('')
    expect(
      resolveEffectivePreviewPin({
        previewArtifact: '',
        artifacts: [newer],
        nodeType: 'research',
        nodeId: 'research',
      }),
    ).toBe('')
    expect(latestOwnNodeHtmlName([newer], '')).toBe('')
  })

  it('keeps known contract products on the pipeline grid and drops process artifacts', () => {
    const run = {
      nodes: [
        { id: 'visual_bqc5', type: 'visual', label: '视觉', position: { x: 0, y: 0 }, config: {} },
        { id: 'react_ymx0', type: 'react', label: '澄清', position: { x: 0, y: 0 }, config: {} },
        { id: 'research', type: 'research', label: '调研', position: { x: 0, y: 0 }, config: {} },
        { id: 'clarify', type: 'react', label: '需求', position: { x: 0, y: 0 }, config: {} },
      ],
    } as unknown as Run
    const research = art({ id: 'r', name: 'research.json', kind: 'json', nodeId: 'research' })
    const requirement = art({
      id: 'c',
      name: 'clarified_requirement.json',
      kind: 'json',
      nodeId: 'clarify',
    })
    const page = art({ id: 'p', name: 'page.html', kind: 'html', nodeId: 'visual_bqc5' })
    const hist = art({
      id: historicalStageArtifactId('visual_bqc5', 1),
      name: 'page.html#iter-1',
      kind: 'html',
      nodeId: 'visual_bqc5',
    })
    const complete = art({ id: 'n', name: 'node_complete.json', kind: 'json', nodeId: 'visual_bqc5' })
    const feedback = art({ id: 'f', name: 'feedback_index.json', kind: 'json', nodeId: 'react_ymx0' })
    const copy = art({ id: 'v', name: visualNodePageName('visual_bqc5'), kind: 'html', nodeId: 'visual_bqc5' })
    const demo = art({ id: 'd', name: 'brand-row-preview.html', kind: 'html', nodeId: 'react_ymx0' })
    const reactPage = art({ id: 'rp', name: 'page.html', kind: 'html', nodeId: 'react_ymx0' })
    const names = filterStageGridArtifacts(
      [research, requirement, page, hist, complete, feedback, copy, demo, reactPage],
      run,
    ).map((a) => a.name)
    expect(names).toEqual(['research.json', 'clarified_requirement.json', 'page.html', 'page.html#iter-1'])
    expect(isKnownStageGridArtifact(complete, run)).toBe(false)
    expect(isKnownStageGridArtifact(demo, run)).toBe(false)
    expect(isKnownStageGridArtifact(page)).toBe(true)
    expect(isKnownStageGridArtifact(reactPage, run)).toBe(false)
    expect(
      stageGridArtifactsWithPin(
        [research, requirement, page, complete, feedback, copy, demo, reactPage],
        run,
        'brand-row-preview.html',
      ).map((a) => a.name),
    ).toEqual(['research.json', 'clarified_requirement.json', 'page.html', 'brand-row-preview.html'])
    expect(
      stageGridArtifactsWithPin([research, requirement, page, demo], run, 'page.html').map((a) => a.name),
    ).toEqual(['research.json', 'clarified_requirement.json', 'page.html'])
  })
})

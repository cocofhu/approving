import { describe, expect, it } from 'vitest'
import {
  isGraphDirty,
  isMetaDirty,
  saveDraftBranch,
  shouldSaveBeforeRun,
  snapshotGraph,
  snapshotMeta,
  toolbarSavedKey,
} from './workflowEditorDirty'
import type { WFEdge, WFNode } from './types'

const nodes: WFNode[] = [
  { id: 'in', type: 'input', label: 'Start', position: { x: 0, y: 0 }, config: {} },
  { id: 'out', type: 'output', label: 'End', position: { x: 1, y: 0 }, config: {} },
]
const edges: WFEdge[] = [{ id: 'e1', source: 'in', target: 'out' }]

function wf(overrides: Partial<{
  nodes: WFNode[]
  edges: WFEdge[]
  name: string
  description: string
  needsRepo: boolean
}> = {}) {
  return {
    nodes: overrides.nodes ?? nodes,
    edges: overrides.edges ?? edges,
    name: overrides.name ?? 'Demo',
    description: overrides.description ?? '',
    needsRepo: overrides.needsRepo ?? false,
  }
}

describe('workflowEditorDirty snapshots', () => {
  it('graph dirty detects node/edge changes but not meta', () => {
    const base = snapshotGraph(wf())
    expect(isGraphDirty(wf(), base)).toBe(false)
    expect(isGraphDirty(wf({ name: 'Other' }), base)).toBe(false)
    const moved = wf({
      nodes: [
        { ...nodes[0], position: { x: 9, y: 9 } },
        nodes[1],
      ],
    })
    expect(isGraphDirty(moved, base)).toBe(true)
  })

  it('meta dirty detects name/description/needsRepo only', () => {
    const base = snapshotMeta(wf())
    expect(isMetaDirty(wf(), base)).toBe(false)
    expect(isMetaDirty(wf({ name: 'Renamed' }), base)).toBe(true)
    expect(isMetaDirty(wf({ description: 'd' }), base)).toBe(true)
    expect(isMetaDirty(wf({ needsRepo: true }), base)).toBe(true)
    const graphOnly = wf({
      nodes: [{ ...nodes[0], label: 'X' }, nodes[1]],
    })
    expect(isMetaDirty(graphOnly, base)).toBe(false)
  })

  it('changing only description marks meta dirty (not graph dirty)', () => {
    const live = wf({ description: '' })
    const metaBase = snapshotMeta(live)
    const graphBase = snapshotGraph(live)
    live.description = 'list subtitle'
    expect(isMetaDirty(live, metaBase)).toBe(true)
    expect(isGraphDirty(live, graphBase)).toBe(false)
    expect(saveDraftBranch(false, true)).toBe('meta')
  })

  it('baseline is a deep clone (mutating live wf does not rewrite baseline)', () => {
    const live = wf()
    const base = snapshotGraph(live)
    live.nodes[0].label = 'mutated'
    expect(isGraphDirty(live, base)).toBe(true)
    expect(base.nodes[0].label).toBe('Start')
  })
})

describe('saveDraftBranch / beforeRunStart / toolbar', () => {
  it('saveDraft three branches', () => {
    expect(saveDraftBranch(false, false)).toBe('noop')
    expect(saveDraftBranch(false, true)).toBe('meta')
    expect(saveDraftBranch(true, false)).toBe('graph')
    expect(saveDraftBranch(true, true)).toBe('graph')
  })

  it('beforeRunStart only saves on graph dirty or missing id', () => {
    expect(shouldSaveBeforeRun(false, false)).toBe(true)
    expect(shouldSaveBeforeRun(true, false)).toBe(false)
    expect(shouldSaveBeforeRun(true, true)).toBe(true)
    // meta-only: skip save
    expect(shouldSaveBeforeRun(true, false)).toBe(false)
  })

  it('toolbar binds only graph dirty', () => {
    expect(toolbarSavedKey(false)).toBe('common.saved.saved')
    expect(toolbarSavedKey(true)).toBe('common.saved.unsaved')
  })
})

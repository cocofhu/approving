import { describe, expect, it } from 'vitest'
import {
  inboxComposerMode,
  pickInboxClarifySession,
  resolveInboxReviewState,
} from './inboxReviewMode'
import type { Run } from '@/lib/shared/types'

function runFixture(partial: Partial<Run> & { nodes?: Run['nodes'] }): Run {
  return {
    id: 'r1',
    workflowId: 'w1',
    workflowName: 'wf',
    status: 'waiting_human',
    trigger: 'manual',
    startedAt: '',
    durationSec: 0,
    progress: 0,
    nodeRuns: {},
    artifacts: [],
    ...partial,
  }
}

describe('pickInboxClarifySession', () => {
  it('prefers clarifyByNode over top-level clarify', () => {
    const run = runFixture({
      clarifyByNode: {
        research: { nodeId: 'research', turns: [], done: false },
      },
      clarify: { nodeId: 'other', turns: [], done: true },
    })
    expect(pickInboxClarifySession(run, 'research')?.nodeId).toBe('research')
  })

  it('falls back to top-level clarify when node matches', () => {
    const run = runFixture({
      clarify: { nodeId: 'research', turns: [], done: false, previewArtifact: 'note.md' },
    })
    expect(pickInboxClarifySession(run, 'research')?.done).toBe(false)
    expect(pickInboxClarifySession(run, 'research')?.previewArtifact).toBe('note.md')
  })

  it('returns null when node does not match', () => {
    const run = runFixture({
      clarify: { nodeId: 'research', turns: [], done: false },
    })
    expect(pickInboxClarifySession(run, 'other')).toBeNull()
  })
})

describe('resolveInboxReviewState', () => {
  const openConv = { nodeId: 'research', turns: [], done: false }
  const doneConv = { nodeId: 'research', turns: [], done: true }

  it('research + open session → reviewActive', () => {
    const run = runFixture({
      nodes: [{ id: 'research', type: 'research', label: 'Research', position: { x: 0, y: 0 }, config: {} }],
    })
    expect(
      resolveInboxReviewState({ type: 'clarify', nodeId: 'research' }, run, openConv),
    ).toEqual({ reviewActive: true, nodeMissing: false })
    expect(inboxComposerMode(true)).toBe('review')
  })

  it('react + open session → clarify (not review)', () => {
    const run = runFixture({
      nodes: [{ id: 'react1', type: 'react', label: 'React', position: { x: 0, y: 0 }, config: {} }],
    })
    expect(
      resolveInboxReviewState(
        { type: 'clarify', nodeId: 'react1' },
        run,
        { nodeId: 'react1', turns: [], done: false },
      ),
    ).toEqual({ reviewActive: false, nodeMissing: false })
    expect(inboxComposerMode(false)).toBe('clarify')
  })

  it('approve + open session → clarify (not review)', () => {
    const run = runFixture({
      nodes: [{ id: 'predev', type: 'approve', label: 'Approve', position: { x: 0, y: 0 }, config: {} }],
    })
    expect(
      resolveInboxReviewState(
        { type: 'clarify', nodeId: 'predev' },
        run,
        { nodeId: 'predev', turns: [], done: false },
      ),
    ).toEqual({ reviewActive: false, nodeMissing: false })
  })

  it('done session → not review', () => {
    const run = runFixture({
      nodes: [{ id: 'plan', type: 'plan', label: 'Plan', position: { x: 0, y: 0 }, config: {} }],
    })
    expect(
      resolveInboxReviewState({ type: 'clarify', nodeId: 'plan' }, run, doneConv),
    ).toEqual({ reviewActive: false, nodeMissing: false })
  })

  it('missing graph node → nodeMissing, not review', () => {
    const run = runFixture({ nodes: [] })
    expect(
      resolveInboxReviewState({ type: 'clarify', nodeId: 'research' }, run, openConv),
    ).toEqual({ reviewActive: false, nodeMissing: true })
  })

  it('gate inbox item never activates review', () => {
    const run = runFixture({
      nodes: [{ id: 'research', type: 'research', label: 'Research', position: { x: 0, y: 0 }, config: {} }],
    })
    expect(
      resolveInboxReviewState({ type: 'gate', nodeId: 'research' } as any, run, openConv),
    ).toEqual({ reviewActive: false, nodeMissing: false })
  })
})

import { describe, expect, it } from 'vitest'
import type { Run } from '@/lib/shared/types'
import {
  artifactsListFingerprint,
  mergeRunChromeFields,
  reactSessionsBusyFingerprint,
  runChromeFingerprint,
  runSnapshotUnchanged,
} from './mergeRunChrome'

function run(over: Partial<Run> = {}): Run {
  return {
    id: 'run-1',
    workflowId: 'wf',
    workflowName: 'wf',
    status: 'running',
    trigger: 'manual',
    startedAt: '2026-01-01T00:00:00Z',
    durationSec: 0,
    progress: 0.2,
    nodeRuns: { n1: { nodeId: 'n1', status: 'running', outputs: {} } },
    artifacts: [{ id: 'a1', name: 'plan.json', kind: 'json', nodeId: 'n1', sizeBytes: 10, createdAt: '', workflowName: '' }],
    reactSessions: {
      n1: { busy: true, waiting: 0, items: [{ id: 'q1', text: 'hello' }] },
    },
    clarify: { nodeId: 'n1', turns: [{ role: 'agent', text: 'streaming', at: 't' }], done: false },
    ...over,
  } as unknown as Run
}

describe('mergeRunChromeFields (g1.1)', () => {
  it('patches status, progress, nodeRuns, artifacts and keeps dialogue buffers', () => {
    const current = run()
    const sessions = current.reactSessions
    const clarify = current.clarify
    const snapshot = run({
      status: 'waiting_human',
      progress: 0.6,
      durationSec: 12,
      nodeRuns: { n1: { nodeId: 'n1', status: 'waiting_human', outputs: {} } },
      artifacts: [
        {
          id: 'a1',
          name: 'plan.json',
          kind: 'json',
          nodeId: 'n1',
          sizeBytes: 99,
          revision: 2,
          updatedAt: 't2',
          createdAt: '',
          workflowName: '',
        },
      ],
      reactSessions: { n1: { busy: false, waiting: 0, items: [] } },
      clarify: { nodeId: 'n1', turns: [], done: true },
    })

    const merged = mergeRunChromeFields(current, snapshot)
    expect(merged.status).toBe('waiting_human')
    expect(merged.progress).toBe(0.6)
    expect(merged.nodeRuns.n1.status).toBe('waiting_human')
    expect(merged.artifacts[0].sizeBytes).toBe(99)
    expect(merged.reactSessions).toBe(sessions)
    expect(merged.clarify).toBe(clarify)
    expect(merged.clarify?.turns[0].text).toBe('streaming')
  })

  it('reuses the artifacts array when the list fingerprint is unchanged (g2.2)', () => {
    const current = run()
    const snapshot = run({
      artifacts: current.artifacts.map((a) => ({ ...a })),
    })
    const merged = mergeRunChromeFields(current, snapshot)
    expect(merged.artifacts).toBe(current.artifacts)
    expect(artifactsListFingerprint(merged.artifacts)).toBe(artifactsListFingerprint(current.artifacts))
  })

  it('runSnapshotUnchanged is true for identical chrome + busy flags', () => {
    const a = run()
    const b = run()
    expect(runChromeFingerprint(a)).toBe(runChromeFingerprint(b))
    expect(reactSessionsBusyFingerprint(a)).toBe(reactSessionsBusyFingerprint(b))
    expect(runSnapshotUnchanged(a, b)).toBe(true)
    expect(runSnapshotUnchanged(a, run({ progress: 0.9 }))).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'
import { lastOutputNodeId } from './runOutputSelection'
import type { Run } from './types'

describe('lastOutputNodeId', () => {
  it('picks the latest started output node', () => {
    const run: Run = {
      id: 'r1',
      workflowId: 'w',
      workflowName: 'W',
      status: 'completed',
      trigger: 'manual',
      startedAt: '2026-01-01T00:00:00Z',
      durationSec: 1,
      progress: 1,
      nodeRuns: {
        out1: { nodeId: 'out1', status: 'completed', startedAt: '2026-01-01T00:00:10Z' },
        out2: { nodeId: 'out2', status: 'completed', startedAt: '2026-01-01T00:00:20Z' },
        plan: { nodeId: 'plan', status: 'completed', startedAt: '2026-01-01T00:00:05Z' },
      },
      artifacts: [],
    }
    const nodes = [
      { id: 'plan', type: 'plan' },
      { id: 'out1', type: 'output' },
      { id: 'out2', type: 'output' },
    ]
    expect(lastOutputNodeId(run, nodes)).toBe('out2')
  })
})

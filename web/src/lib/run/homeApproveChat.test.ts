import { describe, expect, it, vi } from 'vitest'
import { ApproveParkTimeout, findApproveWaitingHuman, waitForApprovePark } from './homeApproveChat'
import type { Run } from '@/lib/shared/types'

describe('findApproveWaitingHuman', () => {
  it('returns the approve node that is waiting_human', () => {
    expect(
      findApproveWaitingHuman({
        status: 'waiting_human',
        nodes: [
          { id: 'in', type: 'input', label: '', position: { x: 0, y: 0 }, config: {} },
          { id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} },
        ],
        nodeRuns: { ap: { nodeId: 'ap', status: 'waiting_human' } },
      }),
    ).toBe('ap')
  })

  it('falls back to the first approve when run is waiting_human', () => {
    expect(
      findApproveWaitingHuman({
        status: 'waiting_human',
        nodes: [{ id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} }],
        nodeRuns: {},
      }),
    ).toBe('ap')
  })
})

describe('waitForApprovePark', () => {
  it('polls until approve is waiting_human then returns', async () => {
    const getRun = vi
      .fn()
      .mockResolvedValueOnce({
        id: 'r1',
        status: 'running',
        nodes: [{ id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} }],
        nodeRuns: {},
      } as Run)
      .mockResolvedValueOnce({
        id: 'r1',
        status: 'waiting_human',
        nodes: [{ id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} }],
        nodeRuns: { ap: { nodeId: 'ap', status: 'waiting_human' } },
      } as Run)
    const sleep = vi.fn(async () => undefined)
    const got = await waitForApprovePark('r1', { getRun, sleep, timeoutMs: 5_000, intervalMs: 1 })
    expect(got.nodeId).toBe('ap')
    expect(getRun).toHaveBeenCalledTimes(2)
  })

  it('throws ApproveParkTimeout when the node never parks', async () => {
    const getRun = vi.fn().mockResolvedValue({
      id: 'r1',
      status: 'running',
      nodes: [{ id: 'ap', type: 'approve', label: '', position: { x: 0, y: 0 }, config: {} }],
      nodeRuns: {},
    } as Run)
    const sleep = vi.fn(async () => undefined)
    await expect(
      waitForApprovePark('r1', { getRun, sleep, timeoutMs: 0, intervalMs: 1 }),
    ).rejects.toBeInstanceOf(ApproveParkTimeout)
  })
})

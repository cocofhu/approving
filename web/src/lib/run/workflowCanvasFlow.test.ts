import { describe, expect, it } from 'vitest'
import {
  flowFingerprint,
  pruneFlowCache,
  reuseFlowElement,
  unchangedIdsKeepIdentity,
  type FlowNodeCacheEntry,
} from './workflowCanvasFlow'

type N = { id: string; selected: boolean; label: string; status?: string }

function buildNodes(
  cache: Map<string, FlowNodeCacheEntry<N>>,
  ids: string[],
  selectedId: string | null,
  statusMap: Record<string, string> = {},
): N[] {
  return ids.map((id) => {
    const status = statusMap[id]
    const selected = selectedId === id
    const fingerprint = flowFingerprint({ id, label: `L-${id}`, status })
    return reuseFlowElement(cache, id, fingerprint, selected, () => ({
      id,
      selected,
      label: `L-${id}`,
      status,
    }))
  })
}

describe('workflowCanvasFlow selection identity', () => {
  it('keeps object identity for nodes whose selected flag did not change', () => {
    const cache = new Map<string, FlowNodeCacheEntry<N>>()
    const ids = ['a', 'b', 'c', 'd']
    const before = buildNodes(cache, ids, 'a')
    const after = buildNodes(cache, ids, 'b')

    expect(unchangedIdsKeepIdentity(before, after, new Set(['a', 'b']))).toBe(true)
    expect(before.find((n) => n.id === 'c')).toBe(after.find((n) => n.id === 'c'))
    expect(before.find((n) => n.id === 'd')).toBe(after.find((n) => n.id === 'd'))
    expect(before.find((n) => n.id === 'a')).not.toBe(after.find((n) => n.id === 'a'))
    expect(before.find((n) => n.id === 'b')).not.toBe(after.find((n) => n.id === 'b'))
    expect(after.find((n) => n.id === 'b')?.selected).toBe(true)
  })

  it('rebuilds a node when status fingerprint changes', () => {
    const cache = new Map<string, FlowNodeCacheEntry<N>>()
    const ids = ['a', 'b']
    const before = buildNodes(cache, ids, 'a', { a: 'running' })
    const after = buildNodes(cache, ids, 'a', { a: 'completed' })
    expect(before.find((n) => n.id === 'a')).not.toBe(after.find((n) => n.id === 'a'))
    expect(before.find((n) => n.id === 'b')).toBe(after.find((n) => n.id === 'b'))
  })

  it('prunes stale cache entries', () => {
    const cache = new Map<string, FlowNodeCacheEntry<N>>()
    buildNodes(cache, ['a', 'b', 'c'], 'a')
    pruneFlowCache(cache, ['a', 'c'])
    expect([...cache.keys()].sort()).toEqual(['a', 'c'])
  })
})

import { describe, expect, it } from 'vitest'
import {
  findInboxItemByKey,
  inboxItemKey,
  inboxQueryKey,
  inboxTripleKey,
  isInboxLeftPendingError,
  pickNextActiveAfterRemove,
} from './inboxActiveSelection'

type Item = { runId: string; nodeId: string; iteration?: number; label: string }

function item(id: string, iteration = 1): Item {
  return { runId: `run-${id}`, nodeId: `node-${id}`, iteration, label: id }
}

describe('pickNextActiveAfterRemove', () => {
  it('middle item → next neighbor', () => {
    const list = [item('a'), item('b'), item('c')]
    const next = pickNextActiveAfterRemove(list, inboxItemKey(list[1]))
    expect(next?.label).toBe('c')
  })

  it('last item → previous neighbor', () => {
    const list = [item('a'), item('b'), item('c')]
    const next = pickNextActiveAfterRemove(list, inboxItemKey(list[2]))
    expect(next?.label).toBe('b')
  })

  it('only item → null', () => {
    const list = [item('solo')]
    expect(pickNextActiveAfterRemove(list, inboxItemKey(list[0]))).toBeNull()
  })

  it('first item → next neighbor', () => {
    const list = [item('a'), item('b')]
    const next = pickNextActiveAfterRemove(list, inboxItemKey(list[0]))
    expect(next?.label).toBe('b')
  })

  it('unknown key falls back to first remaining', () => {
    const list = [item('a'), item('b')]
    const next = pickNextActiveAfterRemove(list, 'missing')
    expect(next?.label).toBe('a')
  })
})

describe('inbox keys', () => {
  it('builds item and triple keys', () => {
    const it = item('x', 2)
    expect(inboxItemKey(it)).toBe('run-x:node-x')
    expect(inboxTripleKey(it)).toBe('run-x:node-x:2')
    expect(inboxTripleKey({ runId: 'r', nodeId: 'n' })).toBe('r:n:1')
  })

  it('builds query keys and finds the matching card', () => {
    expect(inboxQueryKey(' run-x ', ' node-x ')).toBe('run-x:node-x')
    expect(inboxQueryKey('', 'node-x')).toBe('')
    expect(inboxQueryKey('run-x', '')).toBe('')
    const list = [item('a'), item('b')]
    expect(findInboxItemByKey(list, 'run-b:node-b')?.label).toBe('b')
    expect(findInboxItemByKey(list, '')).toBeUndefined()
  })
})

describe('isInboxLeftPendingError', () => {
  it('matches no pending inbox item message', () => {
    expect(isInboxLeftPendingError(new Error('no pending inbox item'))).toBe(true)
    expect(isInboxLeftPendingError(new Error('404 No Pending Inbox Item'))).toBe(true)
    expect(isInboxLeftPendingError(new Error('network down'))).toBe(false)
    expect(isInboxLeftPendingError('no pending inbox item')).toBe(true)
  })
})

import { describe, expect, it } from 'vitest'
import {
  classifyPmTurnError,
  findOrphanUserMessageIds,
  isPmFailKind,
} from './pmTurnState'

describe('pmTurnState', () => {
  it('isPmFailKind accepts the five product kinds', () => {
    expect(isPmFailKind('connection')).toBe(true)
    expect(isPmFailKind('sandbox')).toBe(true)
    expect(isPmFailKind('empty')).toBe(true)
    expect(isPmFailKind('unknown')).toBe(true)
    expect(isPmFailKind('stopped')).toBe(true)
    expect(isPmFailKind('bogus')).toBe(false)
  })

  it('classifyPmTurnError maps AbortError to stopped', () => {
    expect(classifyPmTurnError({ name: 'AbortError', message: 'Aborted' })).toBe('stopped')
  })

  it('classifyPmTurnError respects explicit failKind', () => {
    expect(classifyPmTurnError({ failKind: 'empty' })).toBe('empty')
    expect(classifyPmTurnError({ failKind: 'connection' })).toBe('connection')
  })

  it('classifyPmTurnError infers sandbox/connection from message', () => {
    expect(classifyPmTurnError(new Error('sandbox timeout'))).toBe('sandbox')
    expect(classifyPmTurnError(new Error('ws open timeout'))).toBe('connection')
    expect(classifyPmTurnError(new Error('WebSocket connection failed'))).toBe('connection')
    expect(classifyPmTurnError(new Error('something else'))).toBe('unknown')
  })

  it('findOrphanUserMessageIds skips failed and answered turns', () => {
    const ids = findOrphanUserMessageIds([
      { id: 'u1', role: 'user', status: 'ok' },
      { id: 'a1', role: 'assistant' },
      { id: 'u2', role: 'user', status: 'failed' },
      { id: 'u3', role: 'user', status: 'ok' },
    ])
    expect(ids).toEqual(['u3'])
  })

  it('findConvergableOrphanIds skips draft-covered user turn', async () => {
    const { findConvergableOrphanIds } = await import('./pmTurnState')
    const ids = findConvergableOrphanIds(
      [
        { id: 'u1', role: 'user', status: 'ok' },
        { id: 'u2', role: 'user', status: 'ok' },
      ],
      { draftUserMsgId: 'u2' },
    )
    expect(ids).toEqual(['u1'])
  })

  it('findConvergableOrphanIds skipAll returns empty', async () => {
    const { findConvergableOrphanIds } = await import('./pmTurnState')
    expect(
      findConvergableOrphanIds([{ id: 'u1', role: 'user' }], { skipAll: true }),
    ).toEqual([])
  })

  it('shouldApplyEventSeq rejects duplicates and accepts next', async () => {
    const { shouldApplyEventSeq } = await import('./pmTurnState')
    expect(shouldApplyEventSeq(0, -1)).toBe(true)
    expect(shouldApplyEventSeq(0, 0)).toBe(false)
    expect(shouldApplyEventSeq(1, 0)).toBe(true)
    expect(shouldApplyEventSeq(undefined, 5)).toBe(true)
  })
})

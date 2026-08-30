import { describe, expect, it } from 'vitest'
import { createPendingAcpBuffer, pickAcpRails } from './pendingAcpBuffer'

describe('pickAcpRails', () => {
  it('returns empty rails for missing events', () => {
    expect(pickAcpRails(undefined)).toEqual({ thought: '', message: '' })
    expect(pickAcpRails([])).toEqual({ thought: '', message: '' })
  })

  it('keeps last cumulative thought and message', () => {
    expect(
      pickAcpRails([
        { kind: 'thought', text: 't1', t: 1 },
        { kind: 'thought', text: 't1 extended', t: 2 },
        { kind: 'message', text: 'm1', t: 3 },
        { kind: 'tool_call', text: 'read', t: 4 },
        { kind: 'message', text: 'm1 more', t: 5 },
      ]),
    ).toEqual({ thought: 't1 extended', message: 'm1 more' })
  })

  it('multi-turn seed: current-turn rails must not contain prior-turn text', () => {
    // Server AggregateLastTurnFrames already drops turn1; client must treat
    // seed events as absolute current-turn rails (overwrite, never concat).
    const seed = pickAcpRails([
      { kind: 'thought', text: 'think-2', t: 0 },
      { kind: 'message', text: 'answer-2-partial', t: 1 },
    ])
    expect(seed.message).toBe('answer-2-partial')
    expect(seed.thought).toBe('think-2')
    expect(seed.message).not.toContain('answer-1')
    expect(seed.thought).not.toContain('think-1')
  })
})

describe('createPendingAcpBuffer', () => {
  it('keeps latest cumulative events per nodeId', () => {
    const buf = createPendingAcpBuffer()
    buf.push({
      nodeId: 'n1',
      events: [{ kind: 'thought', text: 'a', t: 1 }],
      busy: true,
    })
    buf.push({
      nodeId: 'n1',
      events: [
        { kind: 'thought', text: 'ab', t: 2 },
        { kind: 'message', text: 'hi', t: 3 },
      ],
      busy: true,
    })
    buf.push({ nodeId: 'n2', events: [{ kind: 'message', text: 'other', t: 1 }] })
    expect(buf.size).toBe(2)
    const all = buf.takeAll()
    expect(all).toHaveLength(2)
    const n1 = all.find((f) => f.nodeId === 'n1')!
    expect(pickAcpRails(n1.events)).toEqual({ thought: 'ab', message: 'hi' })
    expect(buf.size).toBe(0)
  })

  it('empty event frame preserves prior rails but can update busy', () => {
    const buf = createPendingAcpBuffer()
    buf.push({
      nodeId: 'n1',
      events: [{ kind: 'thought', text: 'keep', t: 1 }],
      busy: true,
    })
    buf.push({ nodeId: 'n1', events: [], busy: false })
    const [f] = buf.peekAll()
    expect(pickAcpRails(f.events).thought).toBe('keep')
    expect(f.busy).toBe(false)
  })
})

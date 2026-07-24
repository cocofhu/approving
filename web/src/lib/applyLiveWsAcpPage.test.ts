import { describe, expect, it } from 'vitest'
import { applyLiveWsAcpPage, type LiveWsEventPage } from './applyLiveWsAcpPage'
import { mergeAcpEvents, type MergedAcpEvent } from './mergeAcpEvents'
import type { AcpEvent } from './types'

function ev(partial: Partial<AcpEvent> & Pick<AcpEvent, 'kind'>): AcpEvent {
  return { t: 0, ...partial }
}

function page(
  events: MergedAcpEvent[],
  opts: Partial<LiveWsEventPage> = {},
): LiveWsEventPage {
  return {
    events,
    nextCursor: '',
    hasMore: false,
    live: true,
    ...opts,
  }
}

describe('applyLiveWsAcpPage', () => {
  it('returns null for empty WS frames so busy-only updates do not wipe timeline', () => {
    const prev = page(
      mergeAcpEvents(
        [],
        [
          ev({ t: 0, kind: 'tool_call', title: 'Shell', status: 'completed' }),
          ev({ t: 1, kind: 'message', text: 'done' }),
        ],
        { live: true },
      ),
      { hasMore: false },
    )

    expect(applyLiveWsAcpPage(prev, [])).toBeNull()
    expect(applyLiveWsAcpPage(undefined, [])).toBeNull()
  })

  it('keeps !hasMore timeline when empty frame would previously slice prev to []', () => {
    // Regression: prefix=[] + tailPrev=slice(len)=[] + merge([],[]) wiped the page.
    const settled = mergeAcpEvents(
      [],
      [
        ev({ t: 0, kind: 'tool_call', title: 'Shell', status: 'completed' }),
        ev({ t: 1, kind: 'thought', text: '思考中' }),
      ],
      { live: true },
    )
    const prev = page(settled, { hasMore: false })
    const next = applyLiveWsAcpPage(prev, [])
    expect(next).toBeNull()
    // Caller keeps prev; soft-warn + timeline path stays valid.
    expect(prev.events).toHaveLength(2)
    expect(prev.events[0].title).toBe('Shell')
  })

  it('merges non-empty WS into !hasMore page without dropping settled events', () => {
    const prev = page(
      mergeAcpEvents(
        [],
        [ev({ t: 0, kind: 'message', text: 'hello' })],
        { live: true },
      ),
      { hasMore: false },
    )
    const next = applyLiveWsAcpPage(prev, [
      ev({ t: 0, kind: 'message', text: 'hello' }),
      ev({ t: 1, kind: 'thought', text: '续推' }),
    ])
    expect(next).not.toBeNull()
    expect(next!.events).toHaveLength(2)
    expect(next!.events[1].kind).toBe('thought')
    expect(next!.live).toBe(true)
  })

  it('preserves hasMore prefix history when merging a shorter live tail', () => {
    const older: MergedAcpEvent[] = [
      { t: 0, kind: 'message', text: '更早的历史', stableKey: 'message:0:abc' },
    ]
    const tail = mergeAcpEvents(
      [],
      [ev({ t: 1, kind: 'thought', text: '流式' })],
      { live: true },
    )
    const prev = page([...older, ...tail], {
      hasMore: true,
      nextCursor: 'c1',
    })
    const next = applyLiveWsAcpPage(prev, [
      ev({ t: 1, kind: 'thought', text: '流式 — 更长' }),
    ])
    expect(next).not.toBeNull()
    expect(next!.events[0].text).toBe('更早的历史')
    expect(next!.events[1].text).toBe('流式 — 更长')
    expect(next!.hasMore).toBe(true)
    expect(next!.nextCursor).toBe('c1')
  })

  it('creates a new live page when there is no prior snapshot', () => {
    const next = applyLiveWsAcpPage(undefined, [
      ev({ t: 0, kind: 'message', text: 'first' }),
    ])
    expect(next).not.toBeNull()
    expect(next!.events).toHaveLength(1)
    expect(next!.hasMore).toBe(false)
    expect(next!.live).toBe(true)
  })
})

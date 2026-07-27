import { describe, expect, it } from 'vitest'
import { mergePersistedAndLiveTurns } from './mergeClarifyLiveTurns'
import type { ClarifyTurn } from '@/lib/types'

describe('mergePersistedAndLiveTurns', () => {
  it('appends live when persisted has not caught up', () => {
    const persisted: ClarifyTurn[] = [{ role: 'agent', text: '问', at: 't0' }]
    const live: ClarifyTurn[] = [
      { role: 'human', text: '答', at: 't1' },
      { role: 'agent', text: '流', at: 't2', streaming: true },
    ]
    expect(mergePersistedAndLiveTurns(persisted, live)).toEqual([...persisted, ...live])
  })

  it('dedupes human when props.turns catches up while agent still streaming', () => {
    const persisted: ClarifyTurn[] = [
      { role: 'agent', text: '问', at: 't0' },
      { role: 'human', text: '澄清意见甲', at: 't1' },
    ]
    const live: ClarifyTurn[] = [
      { role: 'human', text: '澄清意见甲', at: 't1-live' },
      { role: 'agent', text: '流式中…', at: 't2', streaming: true },
    ]
    const merged = mergePersistedAndLiveTurns(persisted, live)
    const humans = merged.filter((t) => t.role === 'human' && t.text === '澄清意见甲')
    expect(humans).toHaveLength(1)
    expect(merged[merged.length - 1].streaming).toBe(true)
    expect(merged[merged.length - 1].text).toBe('流式中…')
  })

  it('returns persisted only when live empty', () => {
    const persisted: ClarifyTurn[] = [{ role: 'agent', text: 'x', at: 't0' }]
    expect(mergePersistedAndLiveTurns(persisted, [])).toEqual(persisted)
  })
})

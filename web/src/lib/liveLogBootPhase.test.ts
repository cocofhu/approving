import { describe, expect, it } from 'vitest'
import {
  buildBootStageStates,
  deriveBootPhaseIndex,
  isContainerReady,
  ratchetBootPhaseIndex,
} from './liveLogBootPhase'

describe('liveLogBootPhase', () => {
  it('hides when not running or timeline has content', () => {
    expect(deriveBootPhaseIndex('pending', null, false)).toBeNull()
    expect(deriveBootPhaseIndex('failed', { status: 'creating' }, false)).toBeNull()
    expect(deriveBootPhaseIndex('running', { status: 'running' }, true)).toBeNull()
  })

  it('maps sandbox signals to stages', () => {
    expect(deriveBootPhaseIndex('running', null, false)).toBe(0)
    expect(deriveBootPhaseIndex('running', { status: 'creating', containerStatus: 'creating' }, false)).toBe(0)
    expect(deriveBootPhaseIndex('running', { status: 'creating', containerStatus: 'running' }, false)).toBe(1)
    expect(deriveBootPhaseIndex('running', { status: 'creating', containerStatus: 'up' }, false)).toBe(1)
    expect(deriveBootPhaseIndex('running', { status: 'running', containerStatus: 'running' }, false)).toBe(2)
  })

  it('treats container ready aliases', () => {
    expect(isContainerReady('running')).toBe(true)
    expect(isContainerReady('UP')).toBe(true)
    expect(isContainerReady('not_found')).toBe(false)
  })

  it('ratchets forward only', () => {
    expect(ratchetBootPhaseIndex(null, 0)).toBe(0)
    expect(ratchetBootPhaseIndex(1, 0)).toBe(1)
    expect(ratchetBootPhaseIndex(1, 2)).toBe(2)
    expect(ratchetBootPhaseIndex(2, null)).toBeNull()
  })

  it('builds stage states with timeout on active', () => {
    expect(buildBootStageStates(1, false)).toEqual(['done', 'active', 'pending'])
    expect(buildBootStageStates(1, true)).toEqual(['done', 'timeout', 'pending'])
    expect(buildBootStageStates(0, true)).toEqual(['timeout', 'pending', 'pending'])
  })
})

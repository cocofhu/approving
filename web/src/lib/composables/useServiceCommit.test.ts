import { describe, expect, it } from 'vitest'
import { applyHealthCommit, serviceCommit } from './useServiceCommit'

describe('useServiceCommit', () => {
  it('stores a normalized health commit and clears invalid values', () => {
    applyHealthCommit('ABCDEF1dead')
    expect(serviceCommit.value).toBe('abcdef1')
    applyHealthCommit('unknown')
    expect(serviceCommit.value).toBe('')
    applyHealthCommit(undefined)
    expect(serviceCommit.value).toBe('')
  })
})

import { describe, expect, it } from 'vitest'
import { isClarifyInteractive } from './clarifyInteractive'

describe('isClarifyInteractive', () => {
  it('covers react and approve only', () => {
    expect(isClarifyInteractive('react')).toBe(true)
    expect(isClarifyInteractive('approve')).toBe(true)
    expect(isClarifyInteractive('agent')).toBe(false)
    expect(isClarifyInteractive('plan')).toBe(false)
    expect(isClarifyInteractive('')).toBe(false)
    expect(isClarifyInteractive(undefined)).toBe(false)
  })
})

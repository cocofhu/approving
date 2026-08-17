import { describe, expect, it } from 'vitest'
import { normalizeShortSha } from './serviceCommit'

describe('normalizeShortSha', () => {
  it('truncates a valid SHA to 7 lowercase hex chars', () => {
    expect(normalizeShortSha('B01BB39abcdef')).toBe('b01bb39')
    expect(normalizeShortSha('  deadbeef  ')).toBe('deadbee')
  })

  it('hides empty, non-hex, placeholders, and short values', () => {
    expect(normalizeShortSha('')).toBe('')
    expect(normalizeShortSha('   ')).toBe('')
    expect(normalizeShortSha(undefined)).toBe('')
    expect(normalizeShortSha(null)).toBe('')
    expect(normalizeShortSha('unknown')).toBe('')
    expect(normalizeShortSha('N/A')).toBe('')
    expect(normalizeShortSha('—')).toBe('')
    expect(normalizeShortSha('abc123')).toBe('')
    expect(normalizeShortSha('b01bb39-dirty')).toBe('')
    expect(normalizeShortSha('v1.2.3')).toBe('')
  })
})

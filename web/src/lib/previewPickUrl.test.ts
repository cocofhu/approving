// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { previewPickLabel, previewPickPath } from './previewPickUrl'

describe('previewPickPath', () => {
  it('extracts pathname+search+hash', () => {
    expect(previewPickPath('http://127.0.0.1:5173/settings?tab=1#x')).toBe('/settings?tab=1#x')
  })

  it('returns / for root', () => {
    expect(previewPickPath('http://127.0.0.1:3000/')).toBe('/')
  })

  it('accepts already-relative paths', () => {
    expect(previewPickPath('/app/home')).toBe('/app/home')
  })

  it('returns empty for blank or opaque', () => {
    expect(previewPickPath('')).toBe('')
    expect(previewPickPath('not a url')).toBe('')
  })
})

describe('previewPickLabel', () => {
  it('joins path and selector', () => {
    expect(previewPickLabel('http://127.0.0.1:5173/settings', '#hero', 'div')).toBe(
      '/settings · #hero',
    )
  })

  it('falls back to selector when url missing', () => {
    expect(previewPickLabel('', '#hero', 'div')).toBe('#hero')
  })
})

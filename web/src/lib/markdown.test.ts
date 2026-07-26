import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('dompurify', () => ({
  default: {
    sanitize: (html: string) => html,
  },
}))

import {
  clearMarkdownCache,
  getMarkdownParseCount,
  markdownCacheSize,
  renderMarkdown,
  resetMarkdownParseCount,
} from './markdown'

afterEach(() => {
  clearMarkdownCache()
  resetMarkdownParseCount()
})

describe('renderMarkdown cache', () => {
  it('parses once for identical source and returns the same HTML', () => {
    const src = '# Hello\n\n**world**'
    const a = renderMarkdown(src)
    expect(getMarkdownParseCount()).toBe(1)
    expect(a).toContain('<strong>world</strong>')

    const b = renderMarkdown(src)
    expect(getMarkdownParseCount()).toBe(1)
    expect(b).toBe(a)
    expect(markdownCacheSize()).toBe(1)
  })

  it('parses again when source text changes', () => {
    renderMarkdown('one')
    renderMarkdown('two')
    expect(getMarkdownParseCount()).toBe(2)
    expect(markdownCacheSize()).toBe(2)
  })

  it('evicts oldest entries beyond LRU capacity', () => {
    for (let i = 0; i < 70; i++) {
      renderMarkdown(`doc-${i}`)
    }
    expect(markdownCacheSize()).toBe(64)
    // Oldest should be gone → re-parse
    const before = getMarkdownParseCount()
    renderMarkdown('doc-0')
    expect(getMarkdownParseCount()).toBe(before + 1)
  })
})

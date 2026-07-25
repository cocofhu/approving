import { describe, expect, it } from 'vitest'
import {
  escapeHtml,
  highlightJson,
  isJsonArtifact,
  parseJsonState,
} from './highlightJson'

describe('escapeHtml', () => {
  it('escapes &, <, >, and quotes', () => {
    expect(escapeHtml(`a&b<c>"d"`)).toBe('a&amp;b&lt;c&gt;&quot;d&quot;')
  })
})

describe('highlightJson', () => {
  it('colors keys, strings, numbers, bools, null, and punctuation', () => {
    const src = '{"k":"v", "n":1, "b":true, "z":null}'
    const html = highlightJson(src)
    expect(html).toContain('tok-key')
    expect(html).toContain('tok-str')
    expect(html).toContain('tok-num')
    expect(html).toContain('tok-bool')
    expect(html).toContain('tok-null')
    expect(html).toContain('tok-punc')
    expect(html).toContain('&quot;k&quot;')
    expect(html).toContain('&quot;v&quot;')
  })

  it('does not insert newlines into minify source', () => {
    const src = '{"a":1,"b":2}'
    const html = highlightJson(src)
    expect(html).not.toContain('\n')
    // Assert escaped text content without Incomplete multi-character sanitization
    // (CodeQL #2): do not strip tags via /<[^>]+>/g; check escapeHtml fragments.
    expect(html).toContain('&quot;a&quot;')
    expect(html).toContain('&quot;b&quot;')
    expect(html).not.toContain(src)
    expect(escapeHtml(src)).toContain('&quot;a&quot;')
  })

  it('preserves original whitespace/newlines', () => {
    const src = '{\n  "a": 1\n}'
    const html = highlightJson(src)
    expect(html).toContain('\n')
    expect(html).toContain('  ')
  })

  it('escapes HTML-dangerous characters in values', () => {
    const src = '{"x":"<script>"}'
    const html = highlightJson(src)
    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })
})

describe('parseJsonState', () => {
  it('returns ok + highlighted html for valid JSON', () => {
    const state = parseJsonState('{"a":1}')
    expect(state.ok).toBe(true)
    expect(state.html).toContain('tok-key')
    expect(state.html).toContain('tok-num')
  })

  it('returns fallback plain escaped text for invalid JSON without throwing', () => {
    const raw = '{ "title": "broken,\n status: pending'
    const state = parseJsonState(raw)
    expect(state.ok).toBe(false)
    expect(state.html).toBe(escapeHtml(raw))
    expect(state.html).not.toContain('tok-key')
  })

  it('does not pretty-print minify JSON on success', () => {
    const raw = '{"a":1,"b":2}'
    const state = parseJsonState(raw)
    expect(state.ok).toBe(true)
    expect(state.html).not.toContain('\n')
  })
})

describe('isJsonArtifact', () => {
  it('matches kind===json', () => {
    expect(isJsonArtifact({ kind: 'json', name: 'notes.txt' })).toBe(true)
  })

  it('matches .json name suffix case-insensitively', () => {
    expect(isJsonArtifact({ kind: 'markdown', name: 'plan.JSON' })).toBe(true)
    expect(isJsonArtifact({ kind: 'yaml', name: 'foo.json' })).toBe(true)
  })

  it('rejects non-json kinds without .json name', () => {
    expect(isJsonArtifact({ kind: 'markdown', name: 'readme.md' })).toBe(false)
    expect(isJsonArtifact(null)).toBe(false)
  })
})

describe('ArtifactPreview JSON branch contract', () => {
  it('non-reserved JSON artifacts use parseJsonState HTML, never markdown', () => {
    // Ordinary JSON stays on the source-highlight path. Reserved structured names
    // (e.g. plan.json) are routed to StructuredArtifactView by resolveArtifactPreviewBranch.
    const a = { kind: 'json' as const, name: 'data.json' }
    expect(isJsonArtifact(a)).toBe(true)
    const state = parseJsonState('{"title":"x"}')
    expect(state.ok).toBe(true)
    expect(state.html).toContain('tok-key')
    // Invalid still openable via parseJsonState (no throw)
    expect(() => parseJsonState('{bad')).not.toThrow()
    expect(parseJsonState('{bad').ok).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'
import {
  annotationDedupeKey,
  prepareAnnotation,
  pushAnnotationUnique,
  truncateQuote,
  REVIEW_QUOTE_LIMIT,
} from './reviewQuote'

describe('reviewQuote', () => {
  it('truncates quotes at the soft limit and marks truncated', () => {
    const short = truncateQuote('  hello   world  ')
    expect(short).toEqual({ quote: 'hello world', truncated: false })

    const long = '字'.repeat(REVIEW_QUOTE_LIMIT + 20)
    const cut = truncateQuote(long)
    expect(cut.truncated).toBe(true)
    expect(cut.quote).toHaveLength(REVIEW_QUOTE_LIMIT)
  })

  it('dedupes quote by quote+path and field by path', () => {
    expect(
      annotationDedupeKey({ quote: 'a', jsonPath: 'summary' }),
    ).toBe('quote|summary|a')
    expect(annotationDedupeKey({ quote: 'a' })).toBe('quote||a')
    expect(annotationDedupeKey({ jsonPath: 'summary' })).toBe('field|summary')
    expect(annotationDedupeKey({ selector: '#hero' })).toBe('field|#hero')
    expect(annotationDedupeKey({ selector: '#hero', url: 'http://127.0.0.1/a' })).toBe(
      'field|#hero|http://127.0.0.1/a',
    )
  })

  it('allows same selector on different page urls', () => {
    const list: Parameters<typeof pushAnnotationUnique>[0] = []
    expect(
      pushAnnotationUnique(list, { selector: '#hero', url: 'http://127.0.0.1/a', label: 'a' }),
    ).toBe('added')
    expect(
      pushAnnotationUnique(list, { selector: '#hero', url: 'http://127.0.0.1/b', label: 'b' }),
    ).toBe('added')
    expect(
      pushAnnotationUnique(list, { selector: '#hero', url: 'http://127.0.0.1/a', label: 'again' }),
    ).toBe('duplicate')
    expect(list).toHaveLength(2)
  })

  it('allows multiple quotes on the same path and rejects exact duplicates', () => {
    const list: Parameters<typeof pushAnnotationUnique>[0] = []
    expect(pushAnnotationUnique(list, { jsonPath: 'summary', quote: 'one' })).toBe('added')
    expect(pushAnnotationUnique(list, { jsonPath: 'summary', quote: 'two' })).toBe('added')
    expect(pushAnnotationUnique(list, { jsonPath: 'summary', quote: 'one' })).toBe('duplicate')
    expect(list).toHaveLength(2)
  })

  it('keeps whole-field dedupe by path and allows unbound quote-only', () => {
    const list: Parameters<typeof pushAnnotationUnique>[0] = []
    expect(pushAnnotationUnique(list, { jsonPath: 'summary', label: '概述' })).toBe('added')
    expect(pushAnnotationUnique(list, { jsonPath: 'summary', label: 'again' })).toBe('duplicate')
    expect(pushAnnotationUnique(list, { quote: 'orphan excerpt' })).toBe('added')
    expect(list).toHaveLength(2)
    expect(list[1].quote).toBe('orphan excerpt')
    expect(list[1].jsonPath).toBeUndefined()
  })

  it('prepareAnnotation applies soft truncate', () => {
    const prepared = prepareAnnotation({ quote: 'x'.repeat(REVIEW_QUOTE_LIMIT + 5), jsonPath: 'a' })
    expect(prepared.truncated).toBe(true)
    expect(prepared.quote).toHaveLength(REVIEW_QUOTE_LIMIT)
  })
})

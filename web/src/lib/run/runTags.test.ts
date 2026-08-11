import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  MAX_TAG_RUNES,
  isValidRunTag,
  parseTagQuery,
  runeCount,
  serializeTagQuery,
  validateRunTag,
} from './runTags'

describe('validateRunTag / isValidRunTag', () => {
  it('rejects empty', () => {
    expect(validateRunTag('')).toBe('empty')
    expect(validateRunTag('   ')).toBe('empty')
    expect(isValidRunTag('')).toBe(false)
  })

  it('accepts Unicode letters, digits, and _-./', () => {
    expect(isValidRunTag('prod')).toBe(true)
    expect(isValidRunTag('hotfix-api')).toBe(true)
    expect(isValidRunTag('region.cn')).toBe(true)
    expect(isValidRunTag('path/a')).toBe(true)
    expect(isValidRunTag('中文标签')).toBe(true)
    expect(isValidRunTag('owner_alice')).toBe(true)
  })

  it('rejects colon and other illegal characters', () => {
    expect(validateRunTag('owner:alice')).toBe('invalid')
    expect(validateRunTag('a b')).toBe('invalid')
    expect(validateRunTag('a@b')).toBe('invalid')
  })

  it('rejects tags longer than MAX_TAG_RUNES', () => {
    const long = 'a'.repeat(MAX_TAG_RUNES + 1)
    expect(validateRunTag(long)).toBe('too_long')
    expect(runeCount(long)).toBe(MAX_TAG_RUNES + 1)
  })

  it('counts Unicode runes not UTF-16 code units', () => {
    const tag = '标签' + 'a'.repeat(MAX_TAG_RUNES - 2)
    expect(isValidRunTag(tag)).toBe(true)
    expect(isValidRunTag(tag + 'x')).toBe(false)
  })
})

describe('parseTagQuery / serializeTagQuery', () => {
  it('returns empty for empty input', () => {
    expect(parseTagQuery('')).toEqual([])
    expect(serializeTagQuery([])).toBe('')
  })

  it('parses, trims, and dedupes', () => {
    expect(parseTagQuery(' prod , canary ,prod ')).toEqual(['prod', 'canary'])
  })

  it('drops invalid segments', () => {
    expect(parseTagQuery('prod,owner:alice,canary')).toEqual(['prod', 'canary'])
    expect(parseTagQuery('bad tag,!!!')).toEqual([])
  })

  it('round-trips valid tags', () => {
    expect(serializeTagQuery(['hotfix', 'prod'])).toBe('hotfix,prod')
  })
})

describe('useTagFilter URL sync', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    vi.doUnmock('vue-router')
    vi.resetModules()
  })

  it('reads and writes ?tag= via router.replace', async () => {
    const { reactive } = await import('vue')
    const replace = vi.fn()
    const routeState = reactive({ query: { tag: 'prod,canary' } as Record<string, unknown> })
    vi.doMock('vue-router', () => ({
      useRoute: () => routeState,
      useRouter: () => ({
        replace: (loc: { query: Record<string, unknown> }) => {
          routeState.query = { ...loc.query }
          replace(loc)
          return Promise.resolve()
        },
      }),
    }))

    const { useTagFilter } = await import('@/lib/composables/useTagFilter')
    const { selectedTags, addTag, removeTag, toggleTag } = useTagFilter()

    expect(selectedTags.value).toEqual(['prod', 'canary'])

    addTag('hotfix')
    expect(replace).toHaveBeenCalled()
    expect(routeState.query.tag).toBe('prod,canary,hotfix')
    expect(selectedTags.value).toEqual(['prod', 'canary', 'hotfix'])

    removeTag('canary')
    expect(selectedTags.value).toEqual(['prod', 'hotfix'])
    expect(routeState.query.tag).toBe('prod,hotfix')

    toggleTag('prod')
    expect(routeState.query.tag).toBe('hotfix')

    toggleTag('prod')
    expect(routeState.query.tag).toBe('hotfix,prod')

    removeTag('hotfix')
    removeTag('prod')
    expect(routeState.query.tag).toBeUndefined()
  })

  it('ignores invalid tags on add and strips them from URL on read', async () => {
    const { reactive } = await import('vue')
    const replace = vi.fn()
    const routeState = reactive({ query: { tag: 'prod,owner:alice' } as Record<string, unknown> })
    vi.doMock('vue-router', () => ({
      useRoute: () => routeState,
      useRouter: () => ({
        replace: (loc: { query: Record<string, unknown> }) => {
          routeState.query = { ...loc.query }
          replace(loc)
          return Promise.resolve()
        },
      }),
    }))

    const { useTagFilter } = await import('@/lib/composables/useTagFilter')
    const { selectedTags, addTag } = useTagFilter()

    expect(selectedTags.value).toEqual(['prod'])
    // immediate watch cleans illegal URL segment
    expect(routeState.query.tag).toBe('prod')
    addTag('bad:tag')
    expect(routeState.query.tag).toBe('prod')
  })
})

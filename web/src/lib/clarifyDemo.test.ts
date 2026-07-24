import { describe, expect, it } from 'vitest'
import {
  demoGridColsClass,
  demoOptionsOf,
  matchSelectedLabels,
  selectedDemoOption,
  useSideBySide,
} from './clarifyDemo'
import type { ReactQuestion } from './types'

const q: ReactQuestion = {
  id: 'q1',
  prompt: '布局?',
  options: [
    { id: 'a', label: 'A', demoHtml: '<!doctype html><html></html>' },
    { id: 'b', label: 'B' },
    { id: 'c', label: 'C', demoHtml: '  ' },
    { id: 'd', label: 'D', demoHtml: '<!doctype html><html><body>x</body></html>' },
  ],
}

describe('demoOptionsOf', () => {
  it('keeps only non-empty demoHtml', () => {
    expect(demoOptionsOf(q).map((o) => o.id)).toEqual(['a', 'd'])
  })
})

describe('useSideBySide', () => {
  it('is true for 1–3 demos', () => {
    expect(useSideBySide(demoOptionsOf(q))).toBe(true)
  })
  it('is false for >3 demos', () => {
    const many = Array.from({ length: 4 }, (_, i) => ({
      id: `o${i}`,
      label: `L${i}`,
      demoHtml: '<!doctype html><html></html>',
    }))
    expect(useSideBySide(many)).toBe(false)
  })
  it('is false when no demos', () => {
    expect(useSideBySide([])).toBe(false)
  })
})

describe('demoGridColsClass', () => {
  it('adapts column count', () => {
    expect(demoGridColsClass(1)).toContain('grid-cols-1')
    expect(demoGridColsClass(2)).toContain('sm:grid-cols-2')
    expect(demoGridColsClass(3)).toContain('sm:grid-cols-3')
  })
})

describe('matchSelectedLabels', () => {
  it('matches by label', () => {
    const matched = matchSelectedLabels(q, ['A', 'D'])
    expect(matched.map((o) => o.id)).toEqual(['a', 'd'])
  })
  it('returns empty for no match', () => {
    expect(matchSelectedLabels(q, ['Z'])).toEqual([])
  })
})

describe('selectedDemoOption', () => {
  it('returns first selected option with demo', () => {
    const opt = selectedDemoOption(q, ['B', 'D'])
    expect(opt?.id).toBe('d')
  })
  it('returns null when selected has no demo', () => {
    expect(selectedDemoOption(q, ['B'])).toBeNull()
  })
})

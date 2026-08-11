import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClarifyImage } from '../shared/types'
import { clearRunDraft, loadRunDraft, mergeRunDraft, saveRunDraft } from './runDraft'

const WF = 'wf-test-001'

function seed() {
  return {
    inputs: { a: 'default-a', b: 'default-b' },
    images: { p: [] as ClarifyImage[] },
    keys: ['a', 'b', 'p'],
  }
}

describe('mergeRunDraft', () => {
  let store: Record<string, string>

  beforeEach(() => {
    store = {}
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => {
        store[k] = v
      },
      removeItem: (k: string) => {
        delete store[k]
      },
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('merges draft values by key over defaults', () => {
    saveRunDraft(WF, { a: 'draft-a', b: 'draft-b' }, { p: [{ data: 'x', mimeType: 'image/png' }] })
    const { inputs, images, restored } = mergeRunDraft(WF, seed().inputs, seed().images, seed().keys)
    expect(restored).toBe(true)
    expect(inputs.a).toBe('draft-a')
    expect(inputs.b).toBe('draft-b')
    expect(images.p).toEqual([{ data: 'x', mimeType: 'image/png' }])
  })

  it('ignores extra keys in draft not present in current fields', () => {
    saveRunDraft(WF, { a: 'x', removed: 'gone' }, {})
    const { inputs, restored } = mergeRunDraft(WF, seed().inputs, seed().images, ['a', 'b'])
    expect(restored).toBe(true)
    expect(inputs.a).toBe('x')
    expect(inputs).not.toHaveProperty('removed')
    expect(inputs.b).toBe('default-b')
  })

  it('uses defaults for fields missing in draft', () => {
    saveRunDraft(WF, { a: 'only-a' }, {})
    const { inputs, images } = mergeRunDraft(WF, seed().inputs, seed().images, seed().keys)
    expect(inputs.b).toBe('default-b')
    expect(images.p).toEqual([])
  })

  it('silently degrades when stored JSON is corrupt', () => {
    store[`run-draft:${WF}`] = '{not json'
    const { inputs, restored } = mergeRunDraft(WF, seed().inputs, seed().images, seed().keys)
    expect(restored).toBe(false)
    expect(inputs).toEqual(seed().inputs)
  })

  it('allows empty values to overwrite existing draft content', () => {
    saveRunDraft(WF, { a: 'filled' }, {})
    const result = saveRunDraft(WF, { a: '', b: '' }, { p: [] })
    expect(result).toBe('ok')
    const draft = loadRunDraft(WF)
    expect(draft?.inputs.a).toBe('')
    expect(draft?.inputs.b).toBe('')
  })

  it('returns restored=false when no draft exists', () => {
    const { restored } = mergeRunDraft(WF, seed().inputs, seed().images, seed().keys)
    expect(restored).toBe(false)
  })

  it('clearRunDraft removes stored draft', () => {
    saveRunDraft(WF, { a: 'x' }, {})
    clearRunDraft(WF)
    expect(loadRunDraft(WF)).toBeNull()
  })
})

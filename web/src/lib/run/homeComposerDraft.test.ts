import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  HOME_COMPOSER_DRAFT_KEY,
  clearHomeComposerDraft,
  loadHomeComposerDraft,
  saveHomeComposerDraft,
} from './homeComposerDraft'

describe('homeComposerDraft', () => {
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

  it('saves and loads a full draft payload (plan g1.1 / g1.2)', () => {
    const result = saveHomeComposerDraft(
      '做登录页',
      [{ data: 'abc', mimeType: 'image/png', name: 'shot.png' }],
      'wf-ap',
    )
    expect(result).toBe('ok')
    const draft = loadHomeComposerDraft()
    expect(draft).toMatchObject({
      schemaVersion: '1',
      pipelineId: 'wf-ap',
      text: '做登录页',
      attachments: [{ data: 'abc', mimeType: 'image/png', name: 'shot.png' }],
    })
    expect(typeof draft?.savedAt).toBe('number')
    expect(store[HOME_COMPOSER_DRAFT_KEY]).toBeTruthy()
  })

  it('clears instead of writing an empty shell (plan g1.3)', () => {
    saveHomeComposerDraft('x', [], 'wf-ap')
    expect(loadHomeComposerDraft()).not.toBeNull()
    const result = saveHomeComposerDraft('   ', [], 'wf-ap')
    expect(result).toBe('ok')
    expect(loadHomeComposerDraft()).toBeNull()
    expect(store[HOME_COMPOSER_DRAFT_KEY]).toBeUndefined()
  })

  it('keeps draft when only attachments remain', () => {
    saveHomeComposerDraft('', [{ data: 'x', mimeType: 'image/png' }], 'wf-ap')
    expect(loadHomeComposerDraft()?.attachments).toHaveLength(1)
  })

  it('returns null for corrupt JSON (plan g1.2)', () => {
    store[HOME_COMPOSER_DRAFT_KEY] = '{not json'
    expect(loadHomeComposerDraft()).toBeNull()
  })

  it('returns null for missing required fields', () => {
    store[HOME_COMPOSER_DRAFT_KEY] = JSON.stringify({ text: 'ok' })
    expect(loadHomeComposerDraft()).toBeNull()
  })

  it('clearHomeComposerDraft removes the key', () => {
    saveHomeComposerDraft('x', [], 'wf-ap')
    clearHomeComposerDraft()
    expect(loadHomeComposerDraft()).toBeNull()
  })

  it('returns quota_exceeded when setItem throws QuotaExceededError', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => null,
      setItem: () => {
        const err = new Error('quota') as Error & { name: string; code: number }
        err.name = 'QuotaExceededError'
        err.code = 22
        throw err
      },
      removeItem: () => {},
    })
    const result = saveHomeComposerDraft(
      'big',
      [{ data: 'xxxx', mimeType: 'image/png' }],
      'wf-ap',
    )
    expect(result).toBe('quota_exceeded')
  })

  it('falls back to text-only when attachments exceed quota', () => {
    let calls = 0
    const mem: Record<string, string> = {}
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => mem[k] ?? null,
      setItem: (k: string, v: string) => {
        calls++
        if (calls === 1) {
          const err = new Error('quota') as Error & { name: string; code: number }
          err.name = 'QuotaExceededError'
          err.code = 22
          throw err
        }
        mem[k] = v
      },
      removeItem: (k: string) => {
        delete mem[k]
      },
    })
    const result = saveHomeComposerDraft(
      'keep text',
      [{ data: 'huge', mimeType: 'image/png' }],
      'wf-ap',
    )
    expect(result).toBe('quota_exceeded')
    const draft = loadHomeComposerDraft()
    expect(draft?.text).toBe('keep text')
    expect(draft?.attachments).toEqual([])
    expect(draft?.pipelineId).toBe('wf-ap')
  })
})

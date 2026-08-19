// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  STORAGE_KEY,
  __resetSidebarHiddenForTests,
  hydrateSidebarHiddenFromStorage,
  setSidebarHidden,
  sidebarHidden,
} from './sidebarHidden'

describe('sidebarHidden (g1.1 / g1.2)', () => {
  beforeEach(() => {
    __resetSidebarHiddenForTests()
    localStorage.clear()
  })

  afterEach(() => {
    __resetSidebarHiddenForTests()
    vi.unstubAllGlobals()
  })

  it('defaults to expanded (false) and persists approving-sidebar-hidden', () => {
    expect(sidebarHidden.value).toBe(false)
    setSidebarHidden(true)
    expect(sidebarHidden.value).toBe(true)
    expect(localStorage.getItem(STORAGE_KEY)).toBe('true')
    setSidebarHidden(false)
    expect(localStorage.getItem(STORAGE_KEY)).toBe('false')
  })

  it('keeps in-memory toggle when localStorage.setItem throws (g1.1 persist fail)', () => {
    const store: Record<string, string> = {}
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => store[k] ?? null,
      setItem: () => {
        throw new Error('quota')
      },
      removeItem: (k: string) => {
        delete store[k]
      },
      clear: () => {
        for (const k of Object.keys(store)) delete store[k]
      },
      key: () => null,
      length: 0,
    } satisfies Storage)

    expect(() => setSidebarHidden(true)).not.toThrow()
    expect(sidebarHidden.value).toBe(true)
  })

  it('does not use isMobile to rewrite the key (g1.2)', () => {
    setSidebarHidden(true)
    expect(localStorage.getItem(STORAGE_KEY)).toBe('true')
    // Crossing 768px must not clear or invert the stored desktop choice.
    expect(sidebarHidden.value).toBe(true)
  })

  it('hydrates remembered hidden without touching the key (g1.2)', () => {
    localStorage.setItem(STORAGE_KEY, 'true')
    hydrateSidebarHiddenFromStorage()
    expect(sidebarHidden.value).toBe(true)
    expect(localStorage.getItem(STORAGE_KEY)).toBe('true')
  })
})

// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  OUTER_SASH_WIDTH_KEY_SHARED,
  initOuterSashFromMemory,
  readSharedOuterSashMem,
  reviewDefaultRightPx,
  writeSharedOuterSashMem,
} from '@/lib/inbox/reviewLayoutBudget'

const here = dirname(fileURLToPath(import.meta.url))
const orchestrationSrc = readFileSync(join(here, 'useRunDetail.ts'), 'utf8')

describe('useRunDetail outer sash silent restore (plan g2.1)', () => {
  it('orchestration sync-inits from localStorage before mount measure', () => {
    expect(orchestrationSrc).toMatch(/const initialOuterLayout = initOuterSashFromMemory\(initialOuterWs\)/)
    expect(orchestrationSrc).toMatch(/outerRightPx = ref\(initialOuterLayout\.width\)/)
    expect(orchestrationSrc).toMatch(/outerFullOpen = ref\(initialOuterLayout\.fullOpen\)/)
    expect(orchestrationSrc).not.toMatch(/outerRightPx = ref\(0\)/)
  })

  it('re-applies layout when run load finishes (split root becomes visible)', () => {
    expect(orchestrationSrc).toMatch(/watch\(\s*\(\) => runLoading\.value/)
    expect(orchestrationSrc).toMatch(/applyOuterLayout\(\)/)
  })

  it('write then init yields same width — remount path has no default intermediate', () => {
    const store = new Map<string, string>()
    const ls = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => {
        store.set(k, v)
      },
    }
    const orig = globalThis.localStorage
    Object.defineProperty(globalThis, 'localStorage', { value: ls, configurable: true })
    try {
      const ws = 1280
      writeSharedOuterSashMem({ width: 620, fullOpen: false })
      expect(readSharedOuterSashMem()).toEqual({ width: 620, fullOpen: false })
      expect(initOuterSashFromMemory(ws)).toEqual({ width: 620, fullOpen: false })
      store.delete(OUTER_SASH_WIDTH_KEY_SHARED)
      expect(initOuterSashFromMemory(ws).width).toBe(reviewDefaultRightPx(ws))
    } finally {
      Object.defineProperty(globalThis, 'localStorage', { value: orig, configurable: true })
    }
  })

  it('invalid memory falls back to default without throwing', () => {
    const store = new Map<string, string>()
    const ls = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => {
        store.set(k, v)
      },
    }
    const orig = globalThis.localStorage
    Object.defineProperty(globalThis, 'localStorage', { value: ls, configurable: true })
    try {
      store.set(OUTER_SASH_WIDTH_KEY_SHARED, 'not-json')
      const ws = 1200
      expect(initOuterSashFromMemory(ws).width).toBe(reviewDefaultRightPx(ws))
    } finally {
      Object.defineProperty(globalThis, 'localStorage', { value: orig, configurable: true })
    }
  })
})

describe('useRunDetail outer sash tab/mode stability (plan g2.2)', () => {
  it('does not re-read localStorage on nodeTab or viewMode switches', () => {
    expect(orchestrationSrc).not.toMatch(
      /watch\(\s*\(\) => \[desktopOuterSashLayout\.value, nodeTab\.value\]/,
    )
    expect(orchestrationSrc).not.toMatch(/watch\(\s*\(\) => nodeTab\.value/)
    expect(orchestrationSrc).toMatch(/onOuterSashDblClick/)
    expect(orchestrationSrc).toMatch(/persistOuterLayout\(\)/)
  })

  it('dblclick reset writes default back to shared memory', () => {
    expect(orchestrationSrc).toMatch(/outerRightPx\.value = reviewDefaultRightPx\(ws\)/)
    expect(orchestrationSrc).toMatch(/persistOuterLayout\(\)/)
  })
})

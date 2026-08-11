// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/**
 * plan g1 / g3.2 / g3.3 — 全局 .md 折行与 pre 横滚契约（对齐 Demo「修复后」）。
 */
describe('global.css .md wrap contract', () => {
  const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'global.css'), 'utf8')

  it('applies overflow-wrap:anywhere (+ word-break) on .md and .md a (g1.1/f4)', () => {
    expect(css).toMatch(/\.md\s*\{[^}]*overflow-wrap:\s*anywhere/s)
    expect(css).toMatch(/\.md\s*\{[^}]*word-break:\s*break-word/s)
    expect(css).toMatch(/\.md a\s*\{[^}]*overflow-wrap:\s*anywhere/s)
    expect(css).toMatch(/\.md a\s*\{[^}]*word-break:\s*break-word/s)
  })

  it('keeps .md pre horizontal scroll (g1.2/f5)', () => {
    expect(css).toMatch(/\.md pre\s*\{[^}]*overflow-x-auto/s)
    // Must not force-wrap pre (no overflow-wrap on .md pre block itself).
    const preBlock = css.match(/\.md pre\s*\{[^}]*\}/s)?.[0] ?? ''
    expect(preBlock).not.toMatch(/overflow-wrap/)
    expect(preBlock).not.toMatch(/word-break/)
  })
})

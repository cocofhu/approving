import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const css = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '../../styles/global.css'), 'utf8')

describe('global loading reduced-motion (g4.4)', () => {
  it('disables spin, pulse, shimmer, refresh bar, drawer and toast motion', () => {
    expect(css).toMatch(/prefers-reduced-motion:\s*reduce/)
    expect(css).toMatch(/\.app-spinner/)
    expect(css).toMatch(/\.app-skeleton__block::after/)
    expect(css).toMatch(/\.app-refresh-bar/)
    expect(css).toMatch(/\.animate-spin/)
    expect(css).toMatch(/\.animate-pulse/)
    expect(css).toMatch(/\.drawer-fade-enter-active/)
    expect(css).toMatch(/\.toast-enter-active/)
    expect(css).toMatch(/width:\s*100%/)
  })
})

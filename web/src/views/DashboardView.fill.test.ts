// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(join(dir, 'DashboardView.vue'), 'utf8')
const shellSrc = readFileSync(join(dir, '../components/shell/AppShell.vue'), 'utf8')

describe('DashboardView home chat layout', () => {
  it('root uses md+ flex fill without overflow-hidden', () => {
    expect(src).toMatch(/data-testid="dashboard-view"[^>]*class="[^"]*flex flex-col md:h-full md:min-h-0/)
    expect(src).not.toMatch(/data-testid="dashboard-view"[^>]*overflow-hidden/)
    expect(src).not.toMatch(/calc\(100vh/)
  })

  it('centers composer, pipeline cards, and empty states', () => {
    expect(src).toMatch(/data-testid="home-composer"/)
    expect(src).toMatch(/data-testid="home-pipeline-cards"/)
    expect(src).toMatch(/data-testid="home-composer-input"/)
    expect(src).toMatch(/data-testid="home-no-project"/)
    expect(src).toMatch(/data-testid="dashboard-select-project"/)
    expect(src).toMatch(/data-testid="home-pipelines-empty"/)
    expect(src).not.toMatch(/dashboard-kpi-/)
    expect(src).not.toMatch(/dashboard-board-empty/)
    expect(src).not.toMatch(/RunBoardColumn/)
  })

  // plan g1.1 — edge-to-edge stage wash layers
  it('renders full-bleed stage atmosphere layers', () => {
    expect(src).toMatch(/data-testid="home-stage-bg"/)
    expect(src).toMatch(/home-stage__wash/)
    expect(src).toMatch(/home-stage__grid/)
    expect(src).toMatch(/home-stage__glow/)
    expect(src).toMatch(/pointer-events:\s*none/)
  })

  // plan g1.2 — Approving as first visual anchor
  it('sizes brand as the dominant first-screen anchor', () => {
    expect(src).toMatch(/data-testid="home-brand"/)
    expect(src).toMatch(/home-stage__brand/)
    expect(src).toMatch(/clamp\(2\.75rem,\s*9vw,\s*5\.5rem\)/)
    expect(src).toMatch(/font-weight:\s*700/)
    expect(src).toMatch(/var\(--grad-logo\)/)
  })

  // plan g1.3 — floating composer on stage plane
  it('floats composer above the stage with glass surface', () => {
    expect(src).toMatch(/home-stage__composer/)
    expect(src).toMatch(/backdrop-filter:\s*blur\(12px\)/)
    expect(src).toMatch(/data-testid="home-composer"/)
    expect(src).toMatch(/data-testid="home-composer-plus"/)
    expect(src).toMatch(/data-testid="home-pipeline-select"/)
    expect(src).toMatch(/data-testid="home-composer-send"/)
  })

  // plan g2.3 — narrow-screen composer remains operable
  it('adapts composer layout for narrow viewports', () => {
    expect(src).toMatch(/@media \(max-width:\s*520px\)/)
    expect(src).toMatch(/home-stage__input/)
    expect(src).toMatch(/min-width:\s*100%/)
  })

  it('does not alter AppShell height chain', () => {
    expect(shellSrc).toMatch(/h-screen/)
    expect(shellSrc).toMatch(/min-h-0 flex-1/)
  })
})

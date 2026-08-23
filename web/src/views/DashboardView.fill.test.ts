// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(join(dir, 'DashboardView.vue'), 'utf8')
const particleBgSrc = readFileSync(
  join(dir, '../components/dashboard/HomeParticleMeshBackground.vue'),
  'utf8',
)
const shellSrc = readFileSync(join(dir, '../components/shell/AppShell.vue'), 'utf8')
const globalCss = readFileSync(join(dir, '../styles/global.css'), 'utf8')

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
    expect(src).not.toMatch(/data-testid="home-no-project"/)
    expect(src).not.toMatch(/data-testid="dashboard-select-project"/)
    expect(src).toMatch(/data-testid="home-pipelines-empty"/)
    expect(src).toMatch(/data-testid="home-go-projects"/)
    expect(src).not.toMatch(/dashboard-kpi-/)
    expect(src).not.toMatch(/dashboard-board-empty/)
    expect(src).not.toMatch(/RunBoardColumn/)
  })

  // plan g1.1 — no purple stage atmosphere; particle mesh background instead
  it('does not include full-bleed purple stage layers', () => {
    expect(src).not.toMatch(/home-stage-bg/)
    expect(src).not.toMatch(/home-stage__wash/)
    expect(src).not.toMatch(/home-stage__grid/)
    expect(src).not.toMatch(/home-stage__glow/)
    expect(src).not.toMatch(/rgba\(91,\s*66,\s*180/)
    expect(src).toMatch(/HomeParticleMeshBackground/)
    expect(particleBgSrc).toMatch(/data-testid="home-particle-mesh-bg"/)
    expect(particleBgSrc).toMatch(/pointer-events:\s*none/)
  })

  // plan g1.2 / g1.3 — monospace Approving, no gradient shimmer / staggered / serif accent
  it('uses local monospace brand without banned brand effects', () => {
    expect(src).toMatch(/data-testid="home-brand"/)
    expect(src).toMatch(/ui-monospace/)
    expect(src).toMatch(/home-brand__cursor/)
    expect(src).not.toMatch(/var\(--grad-logo\)/)
    expect(src).not.toMatch(/background-clip:\s*text/)
    expect(src).not.toMatch(/shimmer/)
    expect(src).not.toMatch(/stagger/)
    expect(src).not.toMatch(/serif/)
    expect(src).not.toMatch(/fonts\.googleapis|fonts\.gstatic|cdn\.jsdelivr/)
  })

  // plan g1.4 / g2.4 — right-angle Open Design composer with toolbar zone
  it('uses right-angle Open Design composer with toolbar partition', () => {
    expect(src).toMatch(/home-composer/)
    expect(src).toMatch(/home-composer__toolbar/)
    expect(src).toMatch(/home-composer__plus/)
    expect(src).toMatch(/home-composer__send/)
    expect(src).toMatch(/data-testid="home-composer"/)
    expect(src).toMatch(/data-testid="home-composer-plus"/)
    expect(src).toMatch(/data-testid="home-pipeline-select"/)
    expect(src).toMatch(/data-testid="home-composer-send"/)
    expect(src).toMatch(/<textarea/)
  })

  // review v1/v2/v4 — 无 subtitle；流水线卡片脱离全局 .card；附件移除钮直角
  it('omits home-subtitle and forces square pipeline cards', () => {
    expect(src).not.toMatch(/data-testid="home-subtitle"/)
    expect(src).toMatch(/class="home-shell__card[^"]*border border-line/)
    expect(src).not.toMatch(/class="[^"]*\bcard\b[^"]*home-shell__card|class="home-shell__card[^"]*\bcard\b/)
    expect(src).toMatch(/\.home-shell__card\s*\{[^}]*border-radius:\s*0/s)
    expect(src).toMatch(/home-shell__card--selected/)
    expect(src).toMatch(/rounded-none bg-err[\s\S]{0,80}data-testid="home-attach-remove"/)
    expect(src).not.toMatch(/rounded-full bg-err[\s\S]{0,80}data-testid="home-attach-remove"/)
  })

  // plan g2 / g3 — no filter hint; caret opacity settle; placeholder typewriter
  it('omits filter hint and keeps caret settle + placeholder typewriter', () => {
    expect(src).not.toMatch(/data-testid="home-filter-hint"/)
    expect(src).not.toMatch(/filterHint/)
    expect(src).toMatch(/home-brand__cursor--gone/)
    expect(src).toMatch(/data-testid="home-composer-placeholder"/)
    expect(src).toMatch(/shiftKey/)
    expect(src).toMatch(/prefers-reduced-motion/)
  })

  // plan g1 — pipeline rail hides scrollbar and adds edge nav aligned to page.html demo
  it('hides pipeline horizontal scrollbar and adds edge scroll arrows', () => {
    expect(src).toMatch(/data-testid="home-pipeline-rail-wrap"/)
    expect(src).toMatch(/data-testid="home-pipeline-scroll-prev"/)
    expect(src).toMatch(/data-testid="home-pipeline-scroll-next"/)
    expect(src).toMatch(/home-pipeline-rail/)
    expect(src).toMatch(/home-pipeline-rail--overflow/)
    expect(src).toMatch(/justify-content:\s*center/)
    expect(src).toMatch(/home-pipeline-rail--overflow[\s\S]*justify-content:\s*flex-start/)
    expect(src).not.toMatch(
      /data-testid="home-pipeline-cards"[^>]*justify-center/,
    )
    expect(src).toMatch(/scrollbar-width:\s*none/)
    expect(src).toMatch(/::-webkit-scrollbar/)
    expect(src).toMatch(/home-pipeline-nav/)
    expect(src).toMatch(/home-pipeline-fade/)
    expect(src).toMatch(/syncPipelineNav/)
    expect(src).toMatch(/scrollPipelineByDir/)
    expect(src).not.toMatch(
      /data-testid="home-pipeline-cards"[^>]*overflow-x-auto/,
    )
  })

  // plan g2.5 / g3.2 — scoped only; no external fonts
  it('keeps styles scoped to DashboardView and does not load external fonts', () => {
    expect(src).toMatch(/<style scoped>/)
    expect(src).not.toMatch(/@import|googleapis|gstatic|jsdelivr.*font/)
    expect(globalCss).toMatch(/:root|html/)
    expect(src).not.toContain(globalCss.slice(0, 40))
  })

  it('does not alter AppShell height chain', () => {
    expect(shellSrc).toMatch(/h-screen/)
    expect(shellSrc).toMatch(/min-h-0 flex-1/)
  })
})

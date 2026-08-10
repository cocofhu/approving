// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const src = readFileSync(join(dir, 'DashboardView.vue'), 'utf8')
const boardSrc = readFileSync(join(dir, 'BoardView.vue'), 'utf8')
const shellSrc = readFileSync(join(dir, '../components/shell/AppShell.vue'), 'utf8')

describe('DashboardView desktop fill height chain (g1 / g2)', () => {
  it('root uses md+ flex fill without overflow-hidden', () => {
    expect(src).toMatch(/data-testid="dashboard-view"[^>]*class="[^"]*flex flex-col md:h-full md:min-h-0/)
    expect(src).not.toMatch(/data-testid="dashboard-view"[^>]*overflow-hidden/)
    expect(src).not.toMatch(/calc\(100vh/)
  })

  it('keeps title, KPI, and error bar shrink-0', () => {
    expect(src).toMatch(/mb-5 flex shrink-0 items-center justify-between/)
    expect(src).toMatch(/mb-6 grid shrink-0 grid-cols-2/)
    expect(src).toMatch(/mb-4 flex shrink-0 flex-wrap/)
  })

  it('board card flex-1 only when hasProject; empty state stays natural height', () => {
    expect(src).toMatch(
      /:class="hasProject \? 'md:flex md:min-h-0 md:flex-1 md:flex-col md:overflow-hidden' : ''"/,
    )
    expect(src).toMatch(/data-testid="dashboard-board-empty"/)
    expect(src).toMatch(/data-testid="dashboard-select-project"/)
    expect(src).toMatch(/pages\.dashboard\.noProjectBoard/)
    expect(src).toMatch(/pages\.dashboard\.selectProject/)
  })

  it('desktop dual columns stretch; small screen keeps items-start', () => {
    expect(src).toMatch(
      /grid grid-cols-1 items-start gap-3\.5 md:grid-cols-2 md:min-h-0 md:flex-1 md:items-stretch/,
    )
    expect(src).not.toMatch(/grid grid-cols-1 items-stretch/)
  })

  it('passes fill to both overview columns only', () => {
    const fillBindings = src.match(/:fill="true"/g) || []
    expect(fillBindings.length).toBe(2)
    expect(boardSrc).not.toMatch(/:fill(?:="true")?/)
  })

  it('keeps full-board entry, preview drawer, and KPI wiring unchanged', () => {
    expect(src).toMatch(/data-testid="dashboard-view-full-board"/)
    expect(src).toMatch(/RunBoardPreviewDrawer/)
    expect(src).toMatch(/pages\.dashboard\.kpi\.running/)
    expect(src).toMatch(/pages\.dashboard\.kpi\.waitingHuman/)
    expect(src).toMatch(/pages\.dashboard\.kpi\.failed/)
    expect(src).toMatch(/pages\.dashboard\.kpi\.completed/)
    expect(src).toMatch(/query: \{ tab: 'board' \}/)
  })

  it('KPI grid remains shrink-0 after button conversion (g2.2)', () => {
    expect(src).toMatch(/mb-6 grid shrink-0 grid-cols-2 gap-4 md:grid-cols-4/)
    expect(src).toMatch(/type="button"/)
    expect(src).toMatch(/dashboard-kpi-\$\{k\.status\}/)
    expect(src).toMatch(/goKpiRuns\(k\.status\)/)
  })
})

describe('BoardView / AppShell unchanged by dashboard fill (g3.3)', () => {
  it('does not alter AppShell height chain', () => {
    expect(shellSrc).toMatch(/h-screen/)
    expect(shellSrc).toMatch(/min-h-0 flex-1/)
  })

  it('full board still uses default RunBoardColumn without fill', () => {
    expect(boardSrc).toMatch(/<RunBoardColumn/)
    expect(boardSrc).not.toMatch(/:fill/)
  })
})

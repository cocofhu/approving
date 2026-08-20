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
    expect(src).toMatch(/data-testid="home-composer-plus"/)
    expect(src).toMatch(/data-testid="home-attach-input"/)
    expect(src).toMatch(/data-testid="home-pending-attachments"/)
    expect(src).toMatch(/data-testid="home-no-project"/)
    expect(src).toMatch(/data-testid="dashboard-select-project"/)
    expect(src).toMatch(/data-testid="home-pipelines-empty"/)
    expect(src).not.toMatch(/attachSoon/)
    expect(src).not.toMatch(/dashboard-kpi-/)
    expect(src).not.toMatch(/dashboard-board-empty/)
    expect(src).not.toMatch(/RunBoardColumn/)
  })

  it('does not alter AppShell height chain', () => {
    expect(shellSrc).toMatch(/h-screen/)
    expect(shellSrc).toMatch(/min-h-0 flex-1/)
  })
})

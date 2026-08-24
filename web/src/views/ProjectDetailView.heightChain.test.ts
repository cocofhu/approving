// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewsDir = dirname(fileURLToPath(import.meta.url))
const detailSrc = readFileSync(join(viewsDir, 'ProjectDetailView.vue'), 'utf8')
const pmChatSrc = readFileSync(join(viewsDir, '../components/pm/PmLeaderChat.vue'), 'utf8')
const appSrc = readFileSync(join(viewsDir, '../App.vue'), 'utf8')
const globalCss = readFileSync(join(viewsDir, '../styles/global.css'), 'utf8')
const gatesSrc = readFileSync(join(viewsDir, 'GatesInboxView.vue'), 'utf8')
const e2eHarnessSrc = readFileSync(join(viewsDir, '../../e2e/project-detail-main.ts'), 'utf8')

function pmLeaderTabBlock(): string {
  const start = detailSrc.indexOf("tab === 'pmLeader'")
  expect(start, 'missing pmLeader tab block').toBeGreaterThanOrEqual(0)
  const end = detailSrc.indexOf("tab === 'cronJobs'", start)
  expect(end, 'missing end marker after pmLeader tab').toBeGreaterThan(start)
  return detailSrc.slice(start, end)
}

describe('ProjectDetailView route-view-wrap height chain (g2.1 / g3.1)', () => {
  it('root panel uses h-full like GatesInboxView to fill block route-view-wrap (g2.1)', () => {
    expect(detailSrc).toMatch(/class="flex h-full min-h-0 flex-1 flex-col overflow-hidden"/)
    expect(detailSrc).toMatch(/data-testid="project-detail-panel"/)
    expect(gatesSrc).toMatch(/class="flex h-full min-h-0 flex-col"/)
    expect(detailSrc).not.toMatch(
      /class="flex min-h-0 flex-1 flex-col"[\s\S]*data-testid="project-detail-panel"/,
    )
  })

  it('production App.vue wraps routed pages in route-view-wrap with height:100% (g3.1)', () => {
    expect(appSrc).toMatch(/<div class="route-view-wrap">/)
    expect(globalCss).toMatch(/\.route-view-wrap\s*\{[^}]*height:\s*100%/s)
    expect(globalCss).toMatch(/\.route-view-wrap\s*\{[^}]*overflow:\s*hidden/s)
  })

  it('e2e harness mounts RouterView without route-view-wrap (documents gap)', () => {
    expect(e2eHarnessSrc).toMatch(/h\(AppShell, null, \{ default: \(\) => h\(RouterView\) \}\)/)
    expect(e2eHarnessSrc).not.toMatch(/route-view-wrap/)
  })
})

describe('ProjectDetailView pmLeader fill-height chain (g2.2 / g2.3)', () => {
  it('pmLeader tab shell flex-1 min-h-0 to consume tab content area (g2.2)', () => {
    const pm = pmLeaderTabBlock()
    expect(pm).toMatch(/data-testid="project-pm-leader-panel"/)
    expect(pm).toMatch(/class="flex min-h-0 flex-1 flex-col"/)
  })

  it('PmLeaderChat uses flex-1 overflow-hidden with internal message scroller (g2.3)', () => {
    expect(pmChatSrc).toMatch(/class="flex min-h-0 flex-1 overflow-hidden border border-line bg-base"/)
    expect(pmChatSrc).toMatch(/class="scroll-area min-h-0 flex-1 space-y-3 overflow-y-auto p-4"/)
  })
})

describe('ProjectDetailView height fix scope lock (g3.2)', () => {
  it('list B tabs use fill + overflow-y-auto (plan g2.1; no 420px floor)', () => {
    expect(detailSrc).toMatch(/tab === 'cronJobs'[\s\S]*?class="scroll-area flex min-h-0 flex-1 flex-col overflow-y-auto"/)
    expect(detailSrc).toMatch(/tab === 'notify' && project[\s\S]*?class="scroll-area min-h-0 flex-1 overflow-y-auto"/)
    expect(detailSrc).not.toMatch(/tab === 'cronJobs'" class="flex min-h-\[420px\] flex-col"/)
    expect(detailSrc).not.toMatch(/tab === 'notify' && project" class="min-h-\[420px\]"/)
  })

  it('fill-height tabs (meta/audit/variables/sandboxEnv) remain min-h-0 flex-1', () => {
    expect(detailSrc).toMatch(/tab === 'meta'" class="flex min-h-0 flex-1 flex-col"/)
    expect(detailSrc).toMatch(/tab === 'audit'" class="flex min-h-0 flex-1 flex-col"/)
    expect(detailSrc).toMatch(/tab === 'sandboxEnv'" class="flex min-h-0 flex-1 flex-col"/)
    expect(detailSrc).toMatch(/tab === 'variables'" class="flex min-h-0 flex-1 flex-col"/)
  })
})

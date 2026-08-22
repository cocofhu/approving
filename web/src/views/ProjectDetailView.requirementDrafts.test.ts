// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const vueSrc = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'ProjectDetailView.vue'), 'utf8')
const logicSrc = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../lib/project/useProjectDetail.ts'),
  'utf8',
)
const src = `${vueSrc}\n${logicSrc}`

describe('ProjectDetailView requirement drafts tab', () => {
  it('registers requirementDrafts tab id and panel mount', () => {
    expect(src).toMatch(/'requirementDrafts'/)
    expect(src).toMatch(/tabRequirementDrafts/)
    expect(src).toMatch(/RequirementDraftsPanel/)
    expect(src).toMatch(/data-testid="project-requirement-drafts-panel"/)
    expect(src).toMatch(/tab === 'requirementDrafts'/)
    expect(src).toMatch(/draftsPanelRef/)
    expect(src).toMatch(/confirmDraftsLeave/)
    expect(src).toMatch(/onBeforeRouteLeave/)
    expect(src).toMatch(/onBeforeRouteUpdate/)
    expect(src).toMatch(/requestLeave/)
  })
})

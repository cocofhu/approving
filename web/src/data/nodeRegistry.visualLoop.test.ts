import { describe, expect, it } from 'vitest'
import { NODE_DEFS, defaultHumanGateForm, isPageHtmlGateBody } from './nodeRegistry'

/**
 * Official typical templates with visual/HtmlPreview are not shipped as seed
 * graphs in this repo. Alignment for fail→implement / {{vars.preview_issues}} /
 * implement→visual is via NODE_DEFS defaults + GateApproval Issue gating +
 * resume snapshot. Deployed/out-of-repo graphs that baked old prompts or edges
 * need a one-time migration or manual update.
 */
describe('visual HtmlPreview Issue loop defaults (g4)', () => {
  it('implement default prompt references vars.preview_issues', () => {
    const prompt = String(NODE_DEFS.implement.defaults?.prompt || '')
    expect(prompt).toContain('{{vars.preview_issues}}')
  })

  it('human_gate declares preview_issues output for Fail snapshot', () => {
    const keys = (NODE_DEFS.human_gate.outputs || []).map((o) => o.key)
    expect(keys).toContain('preview_issues')
  })

  it('documents warehouse inventory: no seeded official visual workflow graphs', () => {
    expect(NODE_DEFS.visual).toBeTruthy()
    expect(NODE_DEFS.implement).toBeTruthy()
    expect(NODE_DEFS.human_gate).toBeTruthy()
    expect(NODE_DEFS.visual.type).toBe('visual')
    expect(NODE_DEFS.implement.type).toBe('implement')
  })

  it('human_gate default actions use approve/revise (GateApproval also accepts pass/fail)', () => {
    const actions = (NODE_DEFS.human_gate.defaults as { actions?: { id: string }[] }).actions || []
    const ids = actions.map((a) => a.id)
    expect(ids).toContain('approve')
    expect(ids).toContain('revise')
  })

  it('page.html body_template defaults to empty form (aligns with app_preview)', () => {
    expect(isPageHtmlGateBody('{{nodes.visual.outputs.page}}')).toBe(true)
    expect(defaultHumanGateForm('{{nodes.visual.outputs.page}}')).toEqual([])
    expect(defaultHumanGateForm('{{nodes.research.outputs.research}}')).toEqual([
      { key: 'comment', label: '评审意见', required: false },
    ])
  })

  it('app_preview hides gate fields and has no default actions', () => {
    const fieldKeys = (NODE_DEFS.app_preview.fields || []).map((f) => f.key)
    expect(fieldKeys).not.toContain('actions')
    expect(fieldKeys).not.toContain('output_var')
    expect(fieldKeys).not.toContain('form')
    expect(fieldKeys).toContain('direct_preview')
    const defaults = NODE_DEFS.app_preview.defaults as Record<string, unknown>
    expect(defaults.direct_preview).toBe(false)
    expect(defaults.actions).toBeUndefined()
    expect(defaults.output_var).toBeUndefined()
  })
})

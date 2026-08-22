import { describe, expect, it } from 'vitest'
import { NODE_DEFS } from './nodeRegistry'
import { productOutputDefs } from '@/lib/run/productNodeArtifacts'

describe('approve node inspector', () => {
  it('configures skill_profile and timeout', () => {
    expect(NODE_DEFS.approve.fields.map((f) => f.key)).toEqual(['skill_profile', 'timeout'])
    expect(NODE_DEFS.approve.defaults).toEqual({ timeout: 30 })
  })

  it('has no leftover inspector knobs', () => {
    for (const key of ['prompt', 'max_rounds', 'auto_var', 'conditional_prompt']) {
      expect(NODE_DEFS.approve.fields.some((f) => f.key === key)).toBe(false)
      expect(NODE_DEFS.approve.defaults?.[key]).toBeUndefined()
    }
  })

  it('derives outputs from the nodereg manifest', () => {
    expect(NODE_DEFS.approve.outputs).toEqual(
      productOutputDefs('approve', [{ key: 'transcript', desc: 'nodes.approve.outputs.transcript.desc' }]),
    )
    expect(NODE_DEFS.approve.outputs.map((o) => o.key)).toEqual([
      'clarified_requirement',
      'clarified_requirement_json',
      'plan',
      'plan_json',
      'research',
      'research_json',
      'proposals',
      'proposals_json',
      'page',
      'transcript',
    ])
  })
})

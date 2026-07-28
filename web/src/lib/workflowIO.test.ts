import { describe, expect, it } from 'vitest'
import { buildEnvelope, sanitizeFilename, collectSkillProfiles, missingSkillProfiles, skillProfileIssues } from './workflowIO'
import type { WFNode } from './types'

describe('sanitizeFilename', () => {
  it('replaces illegal chars and adds .json', () => {
    expect(sanitizeFilename('CI/CD 流水线')).toBe('CI_CD 流水线.json')
  })
})

describe('buildEnvelope', () => {
  it('strips runtime fields and lifts variables from input config', () => {
    const nodes: WFNode[] = [
      {
        id: 'in',
        type: 'input',
        label: 'Start',
        position: { x: 0, y: 0 },
        config: { variables: [{ name: 'repo_url', type: 'string' }] },
      },
      { id: 'out', type: 'output', label: 'End', position: { x: 0, y: 0 }, config: {} },
    ]
    const env = buildEnvelope(
      { name: 'Demo', description: 'd', needsRepo: true },
      { nodes, edges: [{ id: 'e1', source: 'in', target: 'out' }] },
    )
    expect(env.schemaVersion).toBe(1)
    expect(env.name).toBe('Demo')
    expect(env.graph.variables).toHaveLength(1)
    expect(env.graph.nodes[0].config.variables).toBeUndefined()
    expect(env.exportedAt).toBeTruthy()
  })
})

describe('skill profile helpers', () => {
  const nodes: WFNode[] = [
    { id: 'a', type: 'implement', label: 'I', position: { x: 0, y: 0 }, config: { skill_profile: 'ImplementAgent' } },
    { id: 'b', type: 'input', label: 'In', position: { x: 0, y: 0 }, config: {} },
    { id: 'c', type: 'app_preview', label: 'P', position: { x: 0, y: 0 }, config: { skill_profile: 'PreviewAgent' } },
  ]

  it('collects agent node profiles including app_preview', () => {
    expect(collectSkillProfiles(nodes).sort()).toEqual(['ImplementAgent', 'PreviewAgent'].sort())
  })

  it('reports missing profiles by name', () => {
    expect(missingSkillProfiles(nodes, ['Other'])).toEqual(['ImplementAgent', 'PreviewAgent'])
  })

  it('reports missing or foreign skill profiles for import warn', () => {
    const agents = [
      { name: 'ImplementAgent', projectId: 'alpha' },
      { name: 'PreviewAgent', projectId: 'beta' },
      { name: 'unbound', projectId: '' },
    ]
    const withUnbound: WFNode[] = [
      ...nodes,
      { id: 'd', type: 'agent', label: 'A', position: { x: 0, y: 0 }, config: { skill_profile: 'unbound' } },
      { id: 'e', type: 'plan', label: 'Pl', position: { x: 0, y: 0 }, config: { skill_profile: 'ghost' } },
    ]
    const issues = skillProfileIssues(withUnbound, agents, 'alpha')
    expect(issues).toEqual([
      { name: 'PreviewAgent', reason: 'foreign' },
      { name: 'unbound', reason: 'foreign' },
      { name: 'ghost', reason: 'missing' },
    ])
    // same-project only — no false positive
    expect(skillProfileIssues(
      [{ id: 'a', type: 'implement', label: 'I', position: { x: 0, y: 0 }, config: { skill_profile: 'ImplementAgent' } }],
      agents,
      'alpha',
    )).toEqual([])
  })
})

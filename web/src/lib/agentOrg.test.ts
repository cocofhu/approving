import { describe, expect, it } from 'vitest'
import {
  applyDeleteGroup,
  applyMoveAgent,
  applyRemoveAgentFromGroup,
  buildOrgTreeRows,
  wouldCreateGroupCycle,
  wouldCreateReportingCycle,
  groupPath,
  setAgentMembership,
} from './agentOrg'
import type { AgentOrg } from './api'

const sample: AgentOrg = {
  revision: 1,
  groups: [
    { id: 'eng', name: '工程部' },
    { id: 'des', name: '设计组', parentGroupId: 'eng' },
    { id: 'prod', name: '产品部' },
    { id: 'des2', name: '设计组', parentGroupId: 'prod' },
  ],
  agents: {
    alice: { groupIds: ['eng', 'des'], parentAgent: '' },
    bob: { groupIds: ['des'], parentAgent: 'alice' },
    carol: {},
  },
}

describe('agentOrg helpers', () => {
  it('builds path for duplicate names', () => {
    expect(groupPath(sample, 'des')).toBe('工程部 / 设计组')
    expect(groupPath(sample, 'des2')).toBe('产品部 / 设计组')
  })

  it('detects group cycles', () => {
    expect(wouldCreateGroupCycle(sample, 'eng', 'des')).toBe(true)
    expect(wouldCreateGroupCycle(sample, 'des', 'prod')).toBe(false)
  })

  it('detects reporting cycles', () => {
    const o = setAgentMembership(sample, 'alice', ['eng'], 'bob')
    expect(wouldCreateReportingCycle(o, 'alice', 'bob')).toBe(true)
  })

  it('applies move semantics and ungrouped clear', () => {
    let o = applyMoveAgent(sample, 'alice', 'eng', 'prod')
    expect(o.agents.alice.groupIds?.sort()).toEqual(['des', 'prod'])
    o = applyMoveAgent(o, 'alice', 'prod', '')
    expect(o.agents.alice).toBeUndefined()
  })

  it('removes from one group only and preserves parentAgent', () => {
    // MULTI: peel eng only → still in des
    let o = applyRemoveAgentFromGroup(sample, 'alice', 'eng')
    expect(o.agents.alice.groupIds).toEqual(['des'])
    // last group → Ungrouped, keep parentAgent
    o = applyRemoveAgentFromGroup(sample, 'bob', 'des')
    expect(o.agents.bob.groupIds).toBeUndefined()
    expect(o.agents.bob.parentAgent).toBe('alice')
    // Ungrouped / empty source is a no-op
    expect(applyRemoveAgentFromGroup(sample, 'carol', '__ungrouped__')).toBe(sample)
  })

  it('deletes group with promote rules', () => {
    const o = applyDeleteGroup(sample, 'eng')
    expect(o.groups.find((g) => g.id === 'eng')).toBeUndefined()
    expect(o.groups.find((g) => g.id === 'des')?.parentGroupId).toBeFalsy()
  })

  it('renders multi-group and ungrouped rows', () => {
    const rows = buildOrgTreeRows(sample, ['alice', 'bob', 'carol'], new Set())
    expect(rows.some((r) => r.kind === 'agent' && r.name === 'alice' && r.multi)).toBe(true)
    expect(rows.some((r) => r.kind === 'ungrouped-header')).toBe(true)
    expect(rows.some((r) => r.kind === 'agent' && r.name === 'carol' && r.groupId === '__ungrouped__')).toBe(true)
  })
})

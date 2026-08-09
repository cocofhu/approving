import type { AgentOrg, OrgAgentMembership, OrgGroup } from '@/lib/api'

/** Sidebar partition id for agents with no virtual-group membership. */
export const UNGROUPED_ID = '__ungrouped__'

export function emptyOrg(): AgentOrg {
  return { revision: 0, groups: [], agents: {} }
}

export function newGroupId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return `g_${crypto.randomUUID().replace(/-/g, '')}`
  }
  return `g_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`
}

export function membershipOf(org: AgentOrg, agentName: string): OrgAgentMembership {
  return org.agents?.[agentName] || {}
}

export function groupIdsOf(org: AgentOrg, agentName: string): string[] {
  return [...(membershipOf(org, agentName).groupIds || [])]
}

export function parentOf(org: AgentOrg, agentName: string): string {
  return membershipOf(org, agentName).parentAgent || ''
}

export function groupById(org: AgentOrg): Map<string, OrgGroup> {
  const m = new Map<string, OrgGroup>()
  for (const g of org.groups || []) m.set(g.id, g)
  return m
}

/** Hierarchical path like "工程部 / 设计组" for disambiguating duplicate names. */
export function groupPath(org: AgentOrg, groupId: string): string {
  const byId = groupById(org)
  const parts: string[] = []
  let cur: string | undefined = groupId
  const seen = new Set<string>()
  while (cur && byId.has(cur) && !seen.has(cur)) {
    seen.add(cur)
    const group: OrgGroup | undefined = byId.get(cur)
    if (!group) break
    parts.unshift(group.name)
    cur = group.parentGroupId || undefined
  }
  return parts.join(' / ')
}

export function directMemberCount(org: AgentOrg, groupId: string, agentNames: string[]): number {
  let n = 0
  for (const name of agentNames) {
    if (groupIdsOf(org, name).includes(groupId)) n++
  }
  return n
}

export function ungroupedAgents(org: AgentOrg, agentNames: string[]): string[] {
  return agentNames.filter((n) => groupIdsOf(org, n).length === 0)
}

/** All virtual-group ids in the subtree rooted at rootId (including root). */
export function groupSubtreeIds(org: AgentOrg, rootId: string): Set<string> {
  const ids = new Set<string>()
  if (!rootId) return ids
  ids.add(rootId)
  const groups = org.groups || []
  let added = true
  while (added) {
    added = false
    for (const g of groups) {
      if (g.parentGroupId && ids.has(g.parentGroupId) && !ids.has(g.id)) {
        ids.add(g.id)
        added = true
      }
    }
  }
  return ids
}

/** Agent names that belong to any group in the subtree. */
export function agentNamesInSubtree(org: AgentOrg, rootId: string, agentNames: string[]): string[] {
  const gids = groupSubtreeIds(org, rootId)
  return agentNames.filter((n) => groupIdsOf(org, n).some((id) => gids.has(id)))
}

export function isAgentInGroupSubtree(org: AgentOrg, agentName: string, rootGroupId: string): boolean {
  const ids = groupSubtreeIds(org, rootGroupId)
  return groupIdsOf(org, agentName).some((id) => ids.has(id))
}

/** Would attaching child as descendant of ancestor create a cycle? */
export function wouldCreateGroupCycle(org: AgentOrg, childId: string, newParentId: string): boolean {
  if (!newParentId) return false
  if (childId === newParentId) return true
  const byId = groupById(org)
  let cur: string | undefined = newParentId
  const seen = new Set<string>()
  while (cur && byId.has(cur) && !seen.has(cur)) {
    if (cur === childId) return true
    seen.add(cur)
    cur = byId.get(cur)!.parentGroupId || undefined
  }
  return false
}

/** Would setting parentAgent create a reporting cycle? */
export function wouldCreateReportingCycle(
  org: AgentOrg,
  agentName: string,
  parentAgent: string,
): boolean {
  if (!parentAgent) return false
  if (parentAgent === agentName) return true
  let cur = parentAgent
  const seen = new Set<string>([agentName])
  while (cur) {
    if (seen.has(cur)) return true
    seen.add(cur)
    cur = parentOf(org, cur)
  }
  return false
}

export function applyDeleteGroup(org: AgentOrg, groupId: string): AgentOrg {
  const deleted = (org.groups || []).find((g) => g.id === groupId)
  if (!deleted) return org
  const parentId = deleted.parentGroupId || ''
  const groups = (org.groups || [])
    .filter((g) => g.id !== groupId)
    .map((g) => (g.parentGroupId === groupId ? { ...g, parentGroupId: parentId || undefined } : { ...g }))
  const agents: Record<string, OrgAgentMembership> = {}
  for (const [name, m] of Object.entries(org.agents || {})) {
    let gids = [...(m.groupIds || [])]
    const had = gids.includes(groupId)
    gids = gids.filter((id) => id !== groupId)
    if (had && parentId) gids.push(parentId)
    gids = [...new Set(gids)]
    const next: OrgAgentMembership = { ...m, groupIds: gids }
    if (!next.groupIds?.length) delete next.groupIds
    if (!next.parentAgent) delete next.parentAgent
    if (next.groupIds?.length || next.parentAgent) agents[name] = next
  }
  return { ...org, groups, agents }
}

export function applyMoveAgent(
  org: AgentOrg,
  agentName: string,
  sourceGroupId: string,
  targetGroupId: string,
): AgentOrg {
  const agents = { ...(org.agents || {}) }
  const prev = agents[agentName] || {}
  let gids = [...(prev.groupIds || [])]
  if (!targetGroupId) {
    gids = []
  } else {
    gids = gids.filter((id) => id !== sourceGroupId)
    if (!gids.includes(targetGroupId)) gids.push(targetGroupId)
  }
  const next: OrgAgentMembership = { ...prev, groupIds: gids }
  if (!next.groupIds?.length) delete next.groupIds
  if (!next.parentAgent) delete next.parentAgent
  if (next.groupIds?.length || next.parentAgent) agents[agentName] = next
  else delete agents[agentName]
  return { ...org, agents }
}

/**
 * Remove an agent from a single virtual group only (sidebar「移出本组」).
 * Unlike applyMoveAgent(..., target=''), this does not clear all groups.
 * parentAgent is preserved; empty groupIds → Ungrouped.
 */
export function applyRemoveAgentFromGroup(
  org: AgentOrg,
  agentName: string,
  sourceGroupId: string,
): AgentOrg {
  if (!sourceGroupId || sourceGroupId === UNGROUPED_ID) return org
  const agents = { ...(org.agents || {}) }
  const prev = agents[agentName] || {}
  const gids = [...(prev.groupIds || [])].filter((id) => id !== sourceGroupId)
  const next: OrgAgentMembership = { ...prev, groupIds: gids }
  if (!next.groupIds?.length) delete next.groupIds
  if (!next.parentAgent) delete next.parentAgent
  if (next.groupIds?.length || next.parentAgent) agents[agentName] = next
  else delete agents[agentName]
  return { ...org, agents }
}

export function setAgentMembership(
  org: AgentOrg,
  agentName: string,
  groupIds: string[],
  parentAgent: string,
): AgentOrg {
  const agents = { ...(org.agents || {}) }
  const next: OrgAgentMembership = {}
  const gids = [...new Set(groupIds.filter(Boolean))]
  if (gids.length) next.groupIds = gids
  if (parentAgent) next.parentAgent = parentAgent
  if (next.groupIds?.length || next.parentAgent) agents[agentName] = next
  else delete agents[agentName]
  return { ...org, agents }
}

export type OrgTreeRow =
  | {
      kind: 'group'
      key: string
      id: string
      name: string
      depth: number
      count: number
      collapsed: boolean
    }
  | {
      kind: 'agent'
      key: string
      name: string
      groupId: string
      depth: number
      multi: boolean
      parentAgent: string
    }
  | {
      kind: 'ungrouped-header'
      key: string
      depth: number
      count: number
      collapsed: boolean
    }

export function buildOrgTreeRows(
  org: AgentOrg,
  agentNames: string[],
  collapsed: Set<string>,
): OrgTreeRow[] {
  const byId = groupById(org)
  const children = new Map<string, OrgGroup[]>()
  for (const g of org.groups || []) {
    const pid = g.parentGroupId || ''
    if (!children.has(pid)) children.set(pid, [])
    children.get(pid)!.push(g)
  }
  for (const list of children.values()) {
    list.sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
  }

  const membersByGroup = new Map<string, string[]>()
  for (const name of agentNames) {
    for (const gid of groupIdsOf(org, name)) {
      if (!membersByGroup.has(gid)) membersByGroup.set(gid, [])
      membersByGroup.get(gid)!.push(name)
    }
  }
  for (const list of membersByGroup.values()) list.sort((a, b) => a.localeCompare(b))

  const rows: OrgTreeRow[] = []

  function walkGroup(g: OrgGroup, depth: number) {
    const isCollapsed = collapsed.has(g.id)
    const count = (membersByGroup.get(g.id) || []).length
    rows.push({
      kind: 'group',
      key: `group:${g.id}`,
      id: g.id,
      name: g.name,
      depth,
      count,
      collapsed: isCollapsed,
    })
    if (isCollapsed) return
    for (const child of children.get(g.id) || []) walkGroup(child, depth + 1)
    for (const name of membersByGroup.get(g.id) || []) {
      const gids = groupIdsOf(org, name)
      rows.push({
        kind: 'agent',
        key: `agent:${g.id}:${name}`,
        name,
        groupId: g.id,
        depth: depth + 1,
        multi: gids.length >= 2,
        parentAgent: parentOf(org, name),
      })
    }
  }

  for (const root of children.get('') || []) walkGroup(root, 0)
  // Orphan groups whose parent is missing — treat as roots.
  for (const g of org.groups || []) {
    if (g.parentGroupId && !byId.has(g.parentGroupId)) walkGroup(g, 0)
  }

  const ungrouped = ungroupedAgents(org, agentNames)
  const ugCollapsed = collapsed.has(UNGROUPED_ID)
  rows.push({
    kind: 'ungrouped-header',
    key: 'ungrouped',
    depth: 0,
    count: ungrouped.length,
    collapsed: ugCollapsed,
  })
  if (!ugCollapsed) {
    for (const name of ungrouped) {
      rows.push({
        kind: 'agent',
        key: `agent:${UNGROUPED_ID}:${name}`,
        name,
        groupId: UNGROUPED_ID,
        depth: 1,
        multi: false,
        parentAgent: parentOf(org, name),
      })
    }
  }
  return rows
}

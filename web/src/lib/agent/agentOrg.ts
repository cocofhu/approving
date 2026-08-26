import type { AgentOrg, OrgAgentMembership, OrgGroup } from '@/lib/api/api'

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

function membershipOf(org: AgentOrg, agentName: string): OrgAgentMembership {
  return org.agents?.[agentName] || {}
}

export function groupIdsOf(org: AgentOrg, agentName: string): string[] {
  return [...(membershipOf(org, agentName).groupIds || [])]
}

function groupById(org: AgentOrg): Map<string, OrgGroup> {
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

function directMemberCount(org: AgentOrg, groupId: string, agentNames: string[]): number {
  let n = 0
  for (const name of agentNames) {
    if (groupIdsOf(org, name).includes(groupId)) n++
  }
  return n
}

function ungroupedAgents(org: AgentOrg, agentNames: string[]): string[] {
  return agentNames.filter((n) => groupIdsOf(org, n).length === 0)
}

/** All virtual-group ids in the subtree rooted at rootId (including root). */
function groupSubtreeIds(org: AgentOrg, rootId: string): Set<string> {
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
function agentNamesInSubtree(org: AgentOrg, rootGroupId: string, agentNames: string[]): string[] {
  const gids = groupSubtreeIds(org, rootGroupId)
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
    if (next.groupIds?.length) agents[name] = next
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
  if (next.groupIds?.length) agents[agentName] = next
  else delete agents[agentName]
  return { ...org, agents }
}

/**
 * Remove an agent from a single virtual group only (sidebar「移出本组」).
 * Unlike applyMoveAgent(..., target=''), this does not clear all groups.
 * Empty groupIds → Ungrouped.
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
  if (next.groupIds?.length) agents[agentName] = next
  else delete agents[agentName]
  return { ...org, agents }
}

export function setAgentMembership(org: AgentOrg, agentName: string, groupIds: string[]): AgentOrg {
  const agents = { ...(org.agents || {}) }
  const next: OrgAgentMembership = {}
  const gids = [...new Set(groupIds.filter(Boolean))]
  if (gids.length) next.groupIds = gids
  if (next.groupIds?.length) agents[agentName] = next
  else delete agents[agentName]
  return { ...org, agents }
}

export type AgentProjectRef = { name: string; projectId?: string }
export type ProjectNameRef = { id: string; name: string }

/** This group + all descendant group ids (BFS). */
function descendantGroupIds(org: AgentOrg, groupId: string): string[] {
  const byParent = new Map<string, string[]>()
  for (const g of org.groups || []) {
    const pid = g.parentGroupId || ''
    if (!byParent.has(pid)) byParent.set(pid, [])
    byParent.get(pid)!.push(g.id)
  }
  const out: string[] = []
  const seen = new Set<string>()
  const queue = [groupId]
  while (queue.length) {
    const id = queue.shift()!
    if (!id || seen.has(id)) continue
    seen.add(id)
    out.push(id)
    for (const child of byParent.get(id) || []) queue.push(child)
  }
  return out
}

/**
 * Recursive members of a virtual group: this group + all descendants,
 * deduped by Agent name. Shared by assign-project and bracket display.
 */
export function recursiveMemberNames(org: AgentOrg, groupId: string, agentNames: string[]): string[] {
  const ids = new Set(descendantGroupIds(org, groupId))
  const names: string[] = []
  const seen = new Set<string>()
  for (const name of agentNames) {
    if (seen.has(name)) continue
    for (const gid of groupIdsOf(org, name)) {
      if (ids.has(gid)) {
        names.push(name)
        seen.add(name)
        break
      }
    }
  }
  names.sort((a, b) => a.localeCompare(b))
  return names
}

/** Unique non-empty projectId across members, or '' if empty / unbound / mixed. */
export function unifiedProjectId(memberNames: string[], agents: AgentProjectRef[]): string {
  if (!memberNames.length) return ''
  const byName = new Map(agents.map((a) => [a.name, (a.projectId || '').trim()]))
  const ids = new Set<string>()
  for (const name of memberNames) ids.add(byName.get(name) || '')
  if (ids.size !== 1) return ''
  return [...ids][0] || ''
}

/** Display label for a unified projectId; falls back to the id when name is missing. */
export function groupProjectLabel(
  org: AgentOrg,
  groupId: string,
  agentNames: string[],
  agents: AgentProjectRef[],
  projects: ProjectNameRef[],
): string {
  const members = recursiveMemberNames(org, groupId, agentNames)
  const pid = unifiedProjectId(members, agents)
  if (!pid) return ''
  return projects.find((p) => p.id === pid)?.name || pid
}

export function classifyAssignTargets(
  memberNames: string[],
  agents: AgentProjectRef[],
  targetProjectId: string,
): {
  already: string[]
  diffBound: { name: string; oldProjectId: string }[]
  unbound: string[]
} {
  const target = targetProjectId.trim()
  const byName = new Map(agents.map((a) => [a.name, (a.projectId || '').trim()]))
  const already: string[] = []
  const diffBound: { name: string; oldProjectId: string }[] = []
  const unbound: string[] = []
  for (const name of memberNames) {
    const pid = byName.get(name) || ''
    if (pid === target) already.push(name)
    else if (!pid) unbound.push(name)
    else diffBound.push({ name, oldProjectId: pid })
  }
  return { already, diffBound, unbound }
}

export function assignNeedsDraftConfirm(opts: {
  activeName: string
  memberNames: string[]
  draftBindingDirty: boolean
}): boolean {
  if (!opts.activeName || !opts.draftBindingDirty) return false
  return opts.memberNames.includes(opts.activeName)
}

export function shouldSyncDraftAfterAssign(opts: {
  activeName: string
  memberNames: string[]
  failNames: string[]
  syncDraftRequested: boolean
}): boolean {
  if (!opts.syncDraftRequested || !opts.activeName) return false
  if (!opts.memberNames.includes(opts.activeName)) return false
  if (opts.failNames.includes(opts.activeName)) return false
  return true
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
      /** Bracket label when recursive members share one bound project; else empty. */
      projectLabel?: string
    }
  | {
      kind: 'agent'
      key: string
      name: string
      groupId: string
      depth: number
      multi: boolean
    }
  | {
      kind: 'ungrouped-header'
      key: string
      depth: number
      count: number
      collapsed: boolean
    }

/** All virtual-group ids plus UNGROUPED_ID — default collapsed set for a fresh org tree. */
export function allGroupCollapseIds(org: AgentOrg): Set<string> {
  const ids = new Set<string>()
  for (const g of org.groups || []) ids.add(g.id)
  ids.add(UNGROUPED_ID)
  return ids
}

/**
 * Ancestor group ids (including direct memberships) that must be expanded
 * for an agent row to appear in the org tree.
 */
export function ancestorGroupIdsForAgent(org: AgentOrg, agentName: string): Set<string> {
  const byId = groupById(org)
  const out = new Set<string>()
  const gids = groupIdsOf(org, agentName)
  if (!gids.length) {
    out.add(UNGROUPED_ID)
    return out
  }
  for (const start of gids) {
    let cur: string | undefined = start
    const seen = new Set<string>()
    while (cur && byId.has(cur) && !seen.has(cur)) {
      seen.add(cur)
      out.add(cur)
      cur = byId.get(cur)!.parentGroupId || undefined
    }
  }
  return out
}

/** Session-default collapsed set: all groups folded; optional agents' paths expanded. */
export function buildDefaultCollapsedSet(org: AgentOrg, expandAgentNames?: string[]): Set<string> {
  const collapsed = allGroupCollapseIds(org)
  if (!expandAgentNames?.length) return collapsed
  for (const name of expandAgentNames) {
    if (!name) continue
    for (const id of ancestorGroupIdsForAgent(org, name)) collapsed.delete(id)
  }
  return collapsed
}

/**
 * After org structure changes: new ids default collapsed; known ids keep user toggle;
 * optional agents' ancestor paths stay expanded.
 */
export function mergeCollapsedWithOrgChange(
  org: AgentOrg,
  prevCollapsed: Set<string>,
  prevKnownIds: Set<string>,
  expandAgentNames?: string[],
): Set<string> {
  const next = new Set<string>()
  for (const id of allGroupCollapseIds(org)) {
    if (prevKnownIds.has(id)) {
      if (prevCollapsed.has(id)) next.add(id)
    } else {
      next.add(id)
    }
  }
  if (expandAgentNames?.length) {
    for (const name of expandAgentNames) {
      if (!name) continue
      for (const id of ancestorGroupIdsForAgent(org, name)) next.delete(id)
    }
  }
  return next
}

export function buildOrgTreeRows(
  org: AgentOrg,
  agentNames: string[],
  collapsed: Set<string>,
  agents?: AgentProjectRef[],
  projects?: ProjectNameRef[],
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
      projectLabel: agents
        ? groupProjectLabel(org, g.id, agentNames, agents, projects || [])
        : '',
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
      })
    }
  }

  for (const root of children.get('') || []) walkGroup(root, 0)
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
      })
    }
  }
  return rows
}

import type { WFEdge, WFNode, Workflow } from '../shared/types'

const EXPORT_SCHEMA_VERSION = 1

export interface ExportEnvelope {
  schemaVersion: number
  exportedAt: string
  name: string
  description: string
  needsRepo: boolean
  graph: {
    nodes: WFNode[]
    edges: WFEdge[]
    variables: any[]
  }
}

export interface WorkflowGraphPayload {
  nodes: WFNode[]
  edges: WFEdge[]
  variables?: any[]
}

/** Replace illegal filename characters with underscore. */
export function sanitizeFilename(name: string): string {
  return name.replace(/[^\w\u4e00-\u9fff\- ]/g, '_') + '.json'
}

/** Strip runtime fields from nodes before export. */
function exportNodes(nodes: WFNode[]): WFNode[] {
  return nodes.map((n) => {
    const { id, type, label, position, config, checkpoint } = n
    return { id, type, label, position, config: config ? { ...config } : {}, ...(checkpoint ? { checkpoint } : {}) }
  })
}

/** Build a portable export envelope from workflow metadata and graph. */
export function buildEnvelope(
  meta: Pick<Workflow, 'name' | 'description' | 'needsRepo'>,
  graph: WorkflowGraphPayload,
): ExportEnvelope {
  const input = graph.nodes.find((n) => n.type === 'input')
  let variables = graph.variables ?? []
  if (!variables.length && input?.config?.variables) {
    variables = [...(input.config.variables as any[])]
  }
  const nodes = exportNodes(graph.nodes).map((n) => {
    if (n.type !== 'input') return n
    const cfg = { ...n.config }
    delete cfg.variables
    delete cfg.inputs
    return { ...n, config: cfg }
  })
  return {
    schemaVersion: EXPORT_SCHEMA_VERSION,
    exportedAt: new Date().toISOString(),
    name: meta.name,
    description: meta.description ?? '',
    needsRepo: !!meta.needsRepo,
    graph: {
      nodes,
      edges: graph.edges.map((e) => ({ ...e })),
      variables,
    },
  }
}

/** Trigger a browser download of JSON content. */
export function downloadJson(filename: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
}

/** Collect skill_profile references from agent-class nodes. */
export function collectSkillProfiles(nodes: WFNode[]): string[] {
  const agentTypes = new Set([
    'react', 'agent', 'plan', 'implement', 'research', 'test', 'review', 'proposal', 'submit_mr', 'visual', 'app_preview',
  ])
  const out = new Set<string>()
  for (const n of nodes) {
    if (!agentTypes.has(n.type)) continue
    const sp = n.config?.skill_profile
    if (typeof sp === 'string' && sp.trim()) out.add(sp.trim())
  }
  return [...out]
}

export type SkillProfileIssue = {
  name: string
  /** missing = not found / deleted; foreign = unbound or other project */
  reason: 'missing' | 'foreign'
}

/**
 * Return skill_profile refs that are missing or not bound to the workflow project.
 * Unbound Agents (empty projectId) count as foreign.
 */
export function skillProfileIssues(
  nodes: WFNode[],
  agents: { name: string; projectId?: string }[],
  projectId?: string,
): SkillProfileIssue[] {
  const byName = new Map(agents.map((a) => [a.name, a]))
  const pid = String(projectId || '').trim()
  const out: SkillProfileIssue[] = []
  for (const name of collectSkillProfiles(nodes)) {
    const a = byName.get(name)
    if (!a) {
      out.push({ name, reason: 'missing' })
      continue
    }
    const home = String(a.projectId || '').trim()
    if (!home || !pid || home !== pid) {
      out.push({ name, reason: 'foreign' })
    }
  }
  return out
}

/** @deprecated Prefer skillProfileIssues; kept for name-only missing checks in tests. */
export function missingSkillProfiles(nodes: WFNode[], knownAgents: string[]): string[] {
  const known = new Set(knownAgents)
  return collectSkillProfiles(nodes).filter((p) => !known.has(p))
}

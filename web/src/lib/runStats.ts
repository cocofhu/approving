import { NODE_DEFS, nodeColorHex } from '@/data/nodeRegistry'
import type { NodeRun, NodeRunStatus, NodeType, Run, WFNode } from '@/lib/types'

/** Node types that typically pause for human input (wait is baked into durationSec). */
export const HUMAN_WAIT_TYPES: ReadonlySet<NodeType> = new Set([
  'human_gate',
  'react',
  'app_preview',
])

export type SingleDimension = 'process' | 'node' | 'type'
export type MultiDimension = 'node' | 'type'

export interface ProcessAtom {
  nodeId: string
  idxInNode: number
  iteration: number
  status: NodeRunStatus
  startedAt?: string
  durationSec: number
  label: string
  type: NodeType
  hasHumanWait: boolean
  live: boolean
}

export interface StatItem {
  key: string
  label: string
  type: NodeType
  durationSec: number
  sharePct: number | null
  hasHumanWait: boolean
  live: boolean
  status?: NodeRunStatus
  iteration?: number
  count: number
  isProcess: boolean
  color: string
}

export interface SingleRunSummary {
  wallSec: number
  nodeSumSec: number
  gapSec: number
  items: StatItem[]
  bottleneck: StatItem | null
}

export interface MultiRunItem extends StatItem {
  /** Average duration across runs that contain this key (rounded seconds). */
  avgSec: number
  /** Number of selected runs that contribute at least one process for this key. */
  runHits: number
}

export interface MultiRunSummary {
  wallSumSec: number
  processCount: number
  selectedCount: number
  items: MultiRunItem[]
  bottleneck: MultiRunItem | null
}

const ACTIVE_NODE: ReadonlySet<string> = new Set(['running', 'waiting_human'])
const TERMINAL_DUR: ReadonlySet<string> = new Set([
  'completed',
  'failed',
  'skipped',
  'cancelled',
])

/** Share = duration / max(denominator, 1); null when denominator ≤ 0. */
export function sharePct(durationSec: number, denominatorSec: number): number | null {
  if (denominatorSec <= 0) return null
  return Math.round((durationSec / Math.max(denominatorSec, 1)) * 100)
}

export function hasHumanWait(status: NodeRunStatus, type: NodeType): boolean {
  return status === 'waiting_human' || HUMAN_WAIT_TYPES.has(type)
}

/**
 * Elapsed seconds for one StateRun: live nodes use nowMs - startedAt;
 * terminal statuses use durationSec (incl. skipped/cancelled).
 */
export function resolveProcessDuration(
  ex: Pick<NodeRun, 'status' | 'startedAt' | 'durationSec'>,
  nowMs: number,
): number {
  if (ACTIVE_NODE.has(ex.status) && ex.startedAt) {
    const start = Date.parse(ex.startedAt)
    if (!isNaN(start)) return Math.max(0, Math.floor((nowMs - start) / 1000))
    return 0
  }
  if (TERMINAL_DUR.has(ex.status)) {
    return ex.durationSec != null ? Math.max(0, Math.floor(ex.durationSec)) : 0
  }
  return ex.durationSec != null ? Math.max(0, Math.floor(ex.durationSec)) : 0
}

/** Wall-clock for a run: live elapsed from startedAt, else durationSec. */
export function resolveRunWallSec(
  run: Pick<Run, 'status' | 'startedAt' | 'durationSec'>,
  nowMs: number,
): number {
  const start = Date.parse(run.startedAt)
  if (ACTIVE_NODE.has(run.status) || run.status === 'queued') {
    if (!isNaN(start)) return Math.max(0, Math.floor((nowMs - start) / 1000))
  }
  if (run.durationSec > 0) return Math.floor(run.durationSec)
  if (!isNaN(start)) {
    // Fallback already handled by callers via elapsedSec; keep 0 here.
    return 0
  }
  return 0
}

export function compareTimelineOrder(
  a: { startedAt?: string; iteration: number },
  b: { startedAt?: string; iteration: number },
): number {
  const ta = a.startedAt ? Date.parse(a.startedAt) : Infinity
  const tb = b.startedAt ? Date.parse(b.startedAt) : Infinity
  if (ta !== tb) return ta - tb
  return a.iteration - b.iteration
}

/**
 * Default node for Run detail timeline: last running/waiting_human in timeline
 * order, else the last executed entry; undefined when nothing to show.
 */
export function pickDefaultTimelineNodeId(
  run: Pick<Run, 'nodeExecutions' | 'nodeRuns'>,
): string | undefined {
  const execs = run.nodeExecutions || {}
  const nodeRuns = run.nodeRuns || {}
  type Item = { nodeId: string; status: NodeRunStatus; startedAt?: string; iteration: number }
  const items: Item[] = []
  const ids = new Set([...Object.keys(execs), ...Object.keys(nodeRuns)])
  for (const nodeId of ids) {
    const list = execs[nodeId]?.length
      ? execs[nodeId]!
      : nodeRuns[nodeId]
        ? [nodeRuns[nodeId]!]
        : []
    list.forEach((ex, idx) => {
      items.push({
        nodeId,
        status: ex.status,
        startedAt: ex.startedAt,
        iteration: ex.iteration || idx + 1,
      })
    })
  }
  items.sort(compareTimelineOrder)
  if (!items.length) return undefined
  for (let i = items.length - 1; i >= 0; i--) {
    const st = items[i].status
    if (st === 'running' || st === 'waiting_human') return items[i].nodeId
  }
  // "Most recent execution" requires a start timestamp; pending shells must not win.
  for (let i = items.length - 1; i >= 0; i--) {
    if (items[i].startedAt) return items[i].nodeId
  }
  return undefined
}

export function colorForType(type: NodeType, index = 0): string {
  const hex = nodeColorHex(type)
  if (hex) return hex
  const fallback = ['#818CF8', '#38BDF8', '#FBBF24', '#F472B6', '#34D399', '#FB923C']
  return fallback[index % fallback.length]
}

type LabelFn = (label: string | undefined, type: NodeType, nodeId: string) => string

function defaultLabel(label: string | undefined, type: NodeType, nodeId: string): string {
  if (label) return label
  return NODE_DEFS[type]?.label || nodeId
}

/**
 * Flatten every node's execution history into timeline-ordered process atoms.
 * Includes skipped / cancelled; live durations use nowMs.
 */
export function flattenProcesses(
  run: Pick<Run, 'nodeExecutions' | 'nodeRuns'>,
  nodes: WFNode[],
  nowMs: number,
  resolveLabel: LabelFn = defaultLabel,
): ProcessAtom[] {
  const nodeById: Record<string, WFNode> = {}
  for (const n of nodes) nodeById[n.id] = n

  const execs = run.nodeExecutions || {}
  const out: ProcessAtom[] = []

  // Prefer full history; if a live node only exists in nodeRuns, still include it.
  const ids = new Set([...Object.keys(execs), ...Object.keys(run.nodeRuns || {})])
  for (const nodeId of ids) {
    const list = execs[nodeId]?.length ? execs[nodeId]! : run.nodeRuns?.[nodeId] ? [run.nodeRuns[nodeId]!] : []
    const node = nodeById[nodeId]
    const type = (node?.type || 'agent') as NodeType
    const label = resolveLabel(node?.label, type, nodeId)
    list.forEach((ex, idx) => {
      const iteration = ex.iteration || idx + 1
      const durationSec = resolveProcessDuration(ex, nowMs)
      const live = ACTIVE_NODE.has(ex.status)
      out.push({
        nodeId,
        idxInNode: idx,
        iteration,
        status: ex.status,
        startedAt: ex.startedAt,
        durationSec,
        label,
        type,
        hasHumanWait: hasHumanWait(ex.status, type),
        live,
      })
    })
  }

  out.sort(compareTimelineOrder)
  return out
}

export function mergeByNode(processes: ProcessAtom[]): Omit<StatItem, 'sharePct' | 'color'>[] {
  const map = new Map<string, Omit<StatItem, 'sharePct' | 'color'>>()
  const order: string[] = []
  for (const p of processes) {
    const key = p.nodeId
    let cur = map.get(key)
    if (!cur) {
      cur = {
        key,
        label: p.label,
        type: p.type,
        durationSec: 0,
        hasHumanWait: false,
        live: false,
        count: 0,
        isProcess: false,
      }
      map.set(key, cur)
      order.push(key)
    }
    cur.durationSec += p.durationSec
    cur.count += 1
    if (p.hasHumanWait) cur.hasHumanWait = true
    if (p.live) cur.live = true
  }
  return order.map((k) => map.get(k)!)
}

export function mergeByType(processes: ProcessAtom[]): Omit<StatItem, 'sharePct' | 'color'>[] {
  const map = new Map<string, Omit<StatItem, 'sharePct' | 'color'>>()
  const order: string[] = []
  for (const p of processes) {
    const key = p.type
    let cur = map.get(key)
    if (!cur) {
      cur = {
        key,
        label: NODE_DEFS[p.type]?.label || p.type,
        type: p.type,
        durationSec: 0,
        hasHumanWait: false,
        live: false,
        count: 0,
        isProcess: false,
      }
      map.set(key, cur)
      order.push(key)
    }
    cur.durationSec += p.durationSec
    cur.count += 1
    if (p.hasHumanWait) cur.hasHumanWait = true
    if (p.live) cur.live = true
  }
  return order.map((k) => map.get(k)!)
}

function asProcessItems(processes: ProcessAtom[]): Omit<StatItem, 'sharePct' | 'color'>[] {
  return processes.map((p) => ({
    key: `${p.nodeId}#${p.iteration}`,
    label: p.label,
    type: p.type,
    durationSec: p.durationSec,
    hasHumanWait: p.hasHumanWait,
    live: p.live,
    status: p.status,
    iteration: p.iteration,
    count: 1,
    isProcess: true,
  }))
}

function attachShareAndColor(
  items: Omit<StatItem, 'sharePct' | 'color'>[],
  wallSec: number,
): StatItem[] {
  return items.map((it, i) => ({
    ...it,
    sharePct: sharePct(it.durationSec, wallSec),
    color: colorForType(it.type, i),
  }))
}

function pickBottleneck(items: StatItem[]): StatItem | null {
  if (!items.length) return null
  let top = items[0]!
  for (const it of items) {
    if (it.durationSec > top.durationSec) top = it
  }
  return top
}

/** Single-run aggregation for the selected dimension. */
export function aggregateSingleRun(
  run: Pick<Run, 'nodeExecutions' | 'nodeRuns' | 'status' | 'startedAt' | 'durationSec'>,
  nodes: WFNode[],
  dimension: SingleDimension,
  wallSec: number,
  nowMs: number,
  resolveLabel?: LabelFn,
): SingleRunSummary {
  const processes = flattenProcesses(run, nodes, nowMs, resolveLabel)
  const nodeSumSec = processes.reduce((a, p) => a + p.durationSec, 0)
  const gapSec = Math.max(0, wallSec - nodeSumSec)

  let raw: Omit<StatItem, 'sharePct' | 'color'>[]
  if (dimension === 'process') raw = asProcessItems(processes)
  else if (dimension === 'node') raw = mergeByNode(processes)
  else raw = mergeByType(processes)

  const items = attachShareAndColor(raw, wallSec)
  return {
    wallSec,
    nodeSumSec,
    gapSec,
    items,
    bottleneck: pickBottleneck(items),
  }
}

/**
 * Cross-run aggregation. Main metric avgSec = total / runHits (runs that hit the key),
 * matching the approved page.html prototype; count is process occurrence total.
 */
export function aggregateMultiRuns(
  runs: Array<{
    run: Pick<Run, 'id' | 'nodeExecutions' | 'nodeRuns' | 'status' | 'startedAt' | 'durationSec' | 'nodes'>
    wallSec: number
    nodes?: WFNode[]
  }>,
  dimension: MultiDimension,
  nowMs: number,
  resolveLabel?: LabelFn,
): MultiRunSummary {
  const wallSumSec = runs.reduce((a, r) => a + r.wallSec, 0)
  let processCount = 0

  type Acc = {
    key: string
    label: string
    type: NodeType
    durationSec: number
    count: number
    hasHumanWait: boolean
    runIds: Set<string>
  }
  const map = new Map<string, Acc>()
  const order: string[] = []

  for (const { run, nodes: overrideNodes } of runs) {
    const graph = overrideNodes || run.nodes || []
    const processes = flattenProcesses(run, graph, nowMs, resolveLabel)
    processCount += processes.length
    for (const p of processes) {
      const key = dimension === 'node' ? p.nodeId : p.type
      let cur = map.get(key)
      if (!cur) {
        cur = {
          key,
          label: dimension === 'node' ? p.label : NODE_DEFS[p.type]?.label || p.type,
          type: p.type,
          durationSec: 0,
          count: 0,
          hasHumanWait: false,
          runIds: new Set(),
        }
        map.set(key, cur)
        order.push(key)
      }
      cur.durationSec += p.durationSec
      cur.count += 1
      if (p.hasHumanWait) cur.hasHumanWait = true
      cur.runIds.add(run.id)
    }
  }

  const items: MultiRunItem[] = order
    .map((k, i) => {
      const it = map.get(k)!
      const runHits = it.runIds.size
      const avgSec = runHits > 0 ? Math.round(it.durationSec / runHits) : 0
      return {
        key: it.key,
        label: it.label,
        type: it.type,
        durationSec: it.durationSec,
        sharePct: sharePct(it.durationSec, wallSumSec),
        hasHumanWait: it.hasHumanWait,
        live: false,
        count: it.count,
        isProcess: false,
        color: colorForType(it.type, i),
        avgSec,
        runHits,
      }
    })
    .sort((a, b) => b.avgSec - a.avgSec || b.durationSec - a.durationSec)

  return {
    wallSumSec,
    processCount,
    selectedCount: runs.length,
    items,
    bottleneck: items[0] || null,
  }
}

/** Display name helpers for bottleneck / merge badges (caller supplies i18n strings). */
export function bottleneckDisplayName(
  item: StatItem,
  opts: { iterationLabel: (n: number) => string; mergeLabel: (n: number) => string },
): string {
  let name = item.label
  if (item.isProcess && item.iteration != null && item.iteration > 1) {
    name += ` · ${opts.iterationLabel(item.iteration)}`
  }
  if (!item.isProcess && item.count > 1) {
    name += `（${opts.mergeLabel(item.count)}）`
  }
  return name
}

import { ref, type Ref } from 'vue'
import { api, isPaginated } from '@/lib/api'
import { sortRunsByStartedAtDesc } from '@/lib/runBoard'
import type { Run } from '@/lib/types'

export type BoardColumnKey =
  | 'active'
  | 'running'
  | 'waiting_human'
  | 'completed'
  | 'queued'
  | 'failed'
  | 'cancelled'

export type BoardMode = 'dashboard' | 'full'

export interface BoardColumnState {
  key: BoardColumnKey
  items: Run[]
  total: number
  hasMore: boolean
  /** True when server total exceeds returned items (e.g. pageSize capped at 100). */
  truncated: boolean
  /** Set when this column's listRuns request failed. */
  error?: string | null
}

export interface BoardColumnQuery {
  key: BoardColumnKey
  status: string
  pageSize: number
  /** When true, locally re-sort merged multi-status results by startedAt. */
  localSort?: boolean
}

const DASHBOARD_COLUMNS: BoardColumnQuery[] = [
  { key: 'active', status: 'running,waiting_human', pageSize: 5, localSort: true },
  { key: 'completed', status: 'completed', pageSize: 5 },
]

const FULL_MAIN_COLUMNS: BoardColumnQuery[] = [
  { key: 'running', status: 'running', pageSize: 100 },
  { key: 'waiting_human', status: 'waiting_human', pageSize: 100 },
  { key: 'completed', status: 'completed', pageSize: 20 },
]

const EXTRA_COLUMNS: BoardColumnQuery[] = [
  { key: 'queued', status: 'queued', pageSize: 100 },
  { key: 'failed', status: 'failed', pageSize: 100 },
  { key: 'cancelled', status: 'cancelled', pageSize: 100 },
]

function emptyColumn(key: BoardColumnKey): BoardColumnState {
  return { key, items: [], total: 0, hasMore: false, truncated: false, error: null }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return String(err || 'load failed')
}

export type ProjectIdSource = string | Ref<string | null | undefined> | (() => string | null | undefined)

function resolveProjectId(source: ProjectIdSource): string {
  if (typeof source === 'function') return String(source() ?? '').trim()
  if (typeof source === 'object' && source && 'value' in source) return String(source.value ?? '').trim()
  return String(source ?? '').trim()
}

async function fetchColumn(query: BoardColumnQuery, projectId: string): Promise<BoardColumnState> {
  const data = await api.listRuns({
    status: query.status,
    page: 1,
    pageSize: query.pageSize,
    projectId,
  })
  let items = isPaginated(data) ? data.items : data
  const total = isPaginated(data) ? data.total : data.length
  const hasMore = isPaginated(data) ? data.hasMore : false
  if (query.localSort) {
    items = sortRunsByStartedAtDesc(items)
  }
  return {
    key: query.key,
    items,
    total,
    hasMore,
    truncated: total > items.length || hasMore,
    error: null,
  }
}

export interface UseRunBoardOptions {
  mode: BoardMode
  /**
   * Required project boundary. Without a non-empty projectId, load() must not
   * call listRuns (fail-safe: empty columns + missing_project error).
   */
  projectId: ProjectIdSource
  /** Extra statuses to load on the full board (queued / failed / cancelled). */
  extraStatuses?: Ref<Set<string>> | (() => Set<string>)
}

export function useRunBoard(options: UseRunBoardOptions) {
  const columns = ref<Record<string, BoardColumnState>>({})
  const loading = ref(false)
  const hasLoaded = ref(false)
  const error = ref<string | null>(null)
  let requestSeq = 0

  function resolveExtras(): Set<string> {
    if (!options.extraStatuses) return new Set()
    return typeof options.extraStatuses === 'function' ? options.extraStatuses() : options.extraStatuses.value
  }

  function queriesForLoad(): BoardColumnQuery[] {
    if (options.mode === 'dashboard') return DASHBOARD_COLUMNS
    const extras = resolveExtras()
    const extraQs = EXTRA_COLUMNS.filter((q) => extras.has(q.key))
    return [...FULL_MAIN_COLUMNS, ...extraQs]
  }

  async function load() {
    const localSeq = ++requestSeq
    const projectId = resolveProjectId(options.projectId)
    if (!projectId) {
      if (localSeq !== requestSeq) return
      columns.value = {}
      error.value = 'missing_project'
      hasLoaded.value = true
      loading.value = false
      return
    }

    const queries = queriesForLoad()
    loading.value = true
    try {
      const settled = await Promise.all(
        queries.map(async (query) => {
          try {
            return await fetchColumn(query, projectId)
          } catch (err) {
            console.warn('[useRunBoard] listRuns failed', query.key, err)
            const prev = columns.value[query.key]
            return {
              ...(prev ?? emptyColumn(query.key)),
              key: query.key,
              error: errorMessage(err),
            } satisfies BoardColumnState
          }
        }),
      )
      if (localSeq !== requestSeq) return
      const next: Record<string, BoardColumnState> = {}
      const failed: string[] = []
      for (const col of settled) {
        next[col.key] = col
        if (col.error) failed.push(col.key)
      }
      columns.value = next
      error.value = failed.length ? failed.join(',') : null
      hasLoaded.value = true
    } finally {
      if (localSeq === requestSeq) loading.value = false
    }
  }

  function column(key: BoardColumnKey): BoardColumnState {
    return columns.value[key] ?? emptyColumn(key)
  }

  return {
    columns,
    loading,
    hasLoaded,
    error,
    load,
    column,
    DASHBOARD_COLUMNS,
    FULL_MAIN_COLUMNS,
    EXTRA_COLUMNS,
  }
}

export { DASHBOARD_COLUMNS, FULL_MAIN_COLUMNS, EXTRA_COLUMNS }

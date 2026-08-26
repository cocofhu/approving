import type { GlobalTokenStats, TokenStatsWindow } from '@/lib/shared/types'
import { req } from '../httpCore'

export type GlobalTokenStatsParams = {
  window?: TokenStatsWindow | string
  timezone?: string
  utcOffsetMinutes?: number
  source?: 'all' | 'workflow' | 'pm' | string
  projectId?: string
  modelKey?: string
}

export const statsClient = {
  getGlobalTokenStats: (params: GlobalTokenStatsParams, opts?: { signal?: AbortSignal }) => {
    const q = new URLSearchParams()
    q.set('window', params.window || '30d')
    if (params.timezone) q.set('timezone', params.timezone)
    if (params.utcOffsetMinutes != null && Number.isFinite(params.utcOffsetMinutes)) {
      q.set('utcOffsetMinutes', String(Math.round(params.utcOffsetMinutes)))
    }
    if (params.source && params.source !== 'all') q.set('source', params.source)
    if (params.projectId) q.set('projectId', params.projectId)
    if (params.modelKey) q.set('modelKey', params.modelKey)
    return req<GlobalTokenStats>(`/stats/token?${q}`, opts?.signal ? { signal: opts.signal } : undefined)
  },
}

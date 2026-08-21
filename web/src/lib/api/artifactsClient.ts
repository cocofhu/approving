import type { Artifact, InboxItem } from '../shared/types'
import { i18n } from '../shared/i18n'
import {
  isDraining,
  mutationsBlocked,
  showDrainToast,
  shutdownState,
} from '../composables/useShutdownState'
import { apiState, BASE, blobContentUrl, origin, redirectToLogin, req } from './httpCore'
import type { PaginatedResponse } from './apiTypes'

export const artifactsClient = {
  listArtifacts: (params?: {
    page?: number
    pageSize?: number
    wf?: string
    projectId?: string
    q?: string
    /** Opt-in: page by Run (total/pageSize = Run count; items = whole-Run flat list). */
    groupBy?: 'run'
  }) => {
    const qs = new URLSearchParams()
    if (params?.page != null) qs.set('page', String(params.page))
    if (params?.pageSize != null) qs.set('pageSize', String(params.pageSize))
    if (params?.wf) qs.set('wf', params.wf)
    if (params?.projectId) qs.set('projectId', params.projectId)
    if (params?.q) qs.set('q', params.q)
    if (params?.groupBy) qs.set('groupBy', params.groupBy)
    const q = qs.toString()
    const path = q ? `/artifacts?${q}` : '/artifacts'
    if (params?.page != null || params?.pageSize != null) {
      return req<PaginatedResponse<Artifact>>(path)
    }
    return req<Artifact[]>(path)
  },
  artifactContent: (id: string, opts?: { signal?: AbortSignal }) =>
    req<Artifact>(`/artifacts/${id}/content`, opts?.signal ? { signal: opts.signal } : undefined),
  artifactDownloadUrl: (id: string) => `${origin()}/api/artifacts/${id}/download`,
  blobContentUrl,
  // DELETE returns 204 No Content — must not go through req()'s unconditional res.json().
  deleteArtifact: async (id: string): Promise<void> => {
    const path = `/artifacts/${id}`
    if (mutationsBlocked()) {
      const msg = shutdownState.message || i18n.global.t('common.shutdown.notAcceptingRequests')
      showDrainToast(msg)
      throw new Error(msg)
    }
    const res = await fetch(BASE + path, {
      method: 'DELETE',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
    })
    apiState.checked = true
    if (res.status === 204) {
      apiState.online = true
      return
    }
    if (res.status === 401) {
      redirectToLogin()
      throw Object.assign(new Error('unauthorized'), { status: 401 })
    }
    if (res.status === 503) {
      let body: { status?: string; message?: string; error?: string } | null = null
      try {
        body = await res.json()
      } catch {
        // non-JSON 503 body
      }
      if (body?.status === 'shutting_down') {
        const msg = body.message || body.error || i18n.global.t('common.shutdown.notAcceptingRequests')
        showDrainToast(msg)
        throw Object.assign(new Error(msg), { status: 503 })
      }
    }
    if (!isDraining()) apiState.online = false
    let msg = `${res.status} ${path}`
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      // non-JSON error body; keep the status line
    }
    throw Object.assign(new Error(msg), { status: res.status })
  },
}

import type {
  Project,
  ProjectAuditEvent,
  ProjectAuditFacets,
  ProjectAuditStats,
  ProjectTokenStats,
  TokenStatsWindow,
  RequirementDraft,
  RequirementDraftCreateBody,
  RequirementDraftSchedulePatch,
} from '../../shared/types'
import { origin, req } from '../httpCore'
import type { PaginatedResponse } from '../apiTypes'

export const projectsClient = {
  // projects
  listProjects: (opts?: { signal?: AbortSignal }) =>
    req<Project[]>('/projects', opts?.signal ? { signal: opts.signal } : undefined),
  getProject: (id: string) => req<Project>(`/projects/${id}`),
  createProject: (body: Partial<Project>) =>
    req<Project>('/projects', { method: 'POST', body: JSON.stringify(body) }),
  updateProject: (id: string, body: Partial<Project>) =>
    req<Project>(`/projects/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteProject: (id: string) => req<{ status: string }>(`/projects/${id}`, { method: 'DELETE' }),

  listRequirementDrafts: (
    projectId: string,
    params?: { status?: 'open' | 'done' | 'all'; q?: string },
  ) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.q) q.set('q', params.q)
    const qs = q.toString()
    return req<{ items: RequirementDraft[] }>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts${qs ? `?${qs}` : ''}`,
    )
  },
  getRequirementDraft: (projectId: string, draftId: string) =>
    req<RequirementDraft>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts/${encodeURIComponent(draftId)}`,
    ),
  createRequirementDraft: (projectId: string, body?: RequirementDraftCreateBody) =>
    req<RequirementDraft>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts`,
      {
        method: 'POST',
        body: body ? JSON.stringify(body) : undefined,
      },
    ),
  updateRequirementDraft: (
    projectId: string,
    draftId: string,
    body: { title: string; bodyMarkdown?: string },
  ) =>
    req<RequirementDraft>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts/${encodeURIComponent(draftId)}`,
      { method: 'PUT', body: JSON.stringify(body) },
    ),
  patchRequirementDraftStatus: (
    projectId: string,
    draftId: string,
    status: 'open' | 'done',
  ) =>
    req<RequirementDraft>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts/${encodeURIComponent(draftId)}/status`,
      { method: 'PATCH', body: JSON.stringify({ status }) },
    ),
  patchRequirementDraftSchedule: (
    projectId: string,
    draftId: string,
    body: RequirementDraftSchedulePatch,
  ) => {
    const payload: Record<string, unknown> = {}
    if (body.kind !== undefined) payload.kind = body.kind
    if (body.startAt !== undefined) payload.startAt = body.startAt
    if (body.dueAt !== undefined) payload.dueAt = body.dueAt
    if (body.progress !== undefined) payload.progress = body.progress
    if (body.parentId !== undefined) {
      payload.parentId = body.parentId == null ? '' : body.parentId
    }
    return req<RequirementDraft>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts/${encodeURIComponent(draftId)}/schedule`,
      { method: 'PATCH', body: JSON.stringify(payload) },
    )
  },
  deleteRequirementDraft: (projectId: string, draftId: string) =>
    req<{ status: string }>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts/${encodeURIComponent(draftId)}`,
      { method: 'DELETE' },
    ),

  /** Project audit timeline (paginated). Default time window: 24h. */
  listProjectAudit: (
    id: string,
    params?: {
      time?: string
      actor?: string
      callerKind?: string
      action?: string
      resource?: string
      runId?: string
      nodeId?: string
      search?: string
      from?: string
      to?: string
      page?: number
      pageSize?: number
    },
  ) => {
    const q = new URLSearchParams()
    q.set('time', params?.time || '24h')
    if (params?.actor) q.set('actor', params.actor)
    if (params?.callerKind) q.set('callerKind', params.callerKind)
    if (params?.action) q.set('action', params.action)
    if (params?.resource) q.set('resource', params.resource)
    if (params?.runId) q.set('runId', params.runId)
    if (params?.nodeId) q.set('nodeId', params.nodeId)
    if (params?.search) q.set('search', params.search)
    if (params?.from) q.set('from', params.from)
    if (params?.to) q.set('to', params.to)
    q.set('page', String(params?.page ?? 1))
    q.set('pageSize', String(params?.pageSize ?? 20))
    return req<PaginatedResponse<ProjectAuditEvent> & { stats?: ProjectAuditStats }>(
      `/projects/${encodeURIComponent(id)}/audit?${q}`,
    )
  },

  /** Run / node / resource options for dual-mode audit filters. */
  listProjectAuditFacets: (
    id: string,
    params?: {
      time?: string
      runId?: string
      from?: string
      to?: string
    },
  ) => {
    const q = new URLSearchParams()
    q.set('time', params?.time || '24h')
    if (params?.runId) q.set('runId', params.runId)
    if (params?.from) q.set('from', params.from)
    if (params?.to) q.set('to', params.to)
    return req<ProjectAuditFacets>(`/projects/${encodeURIComponent(id)}/audit/facets?${q}`)
  },

  /** Download URL for audit export (JSON); triggers meta-audit on server. */
  exportProjectAuditUrl: (
    id: string,
    params?: {
      format?: 'json' | 'text'
      time?: string
      actor?: string
      callerKind?: string
      action?: string
      resource?: string
      runId?: string
      nodeId?: string
      search?: string
    },
  ) => {
    const q = new URLSearchParams()
    q.set('format', params?.format || 'json')
    q.set('time', params?.time || '24h')
    if (params?.actor) q.set('actor', params.actor)
    if (params?.callerKind) q.set('callerKind', params.callerKind)
    if (params?.action) q.set('action', params.action)
    if (params?.resource) q.set('resource', params.resource)
    if (params?.runId) q.set('runId', params.runId)
    if (params?.nodeId) q.set('nodeId', params.nodeId)
    if (params?.search) q.set('search', params.search)
    return `${origin()}/api/projects/${encodeURIComponent(id)}/audit/export?${q}`
  },

  /** Board Token stats: trend / composition / workflows Top10+other. */
  getProjectTokenStats: (
    id: string,
    params: {
      window?: TokenStatsWindow | string
      timezone?: string
      utcOffsetMinutes?: number
    },
    opts?: { signal?: AbortSignal },
  ) => {
    const q = new URLSearchParams()
    q.set('window', params.window || '30d')
    if (params.timezone) q.set('timezone', params.timezone)
    if (params.utcOffsetMinutes != null && Number.isFinite(params.utcOffsetMinutes)) {
      q.set('utcOffsetMinutes', String(Math.round(params.utcOffsetMinutes)))
    }
    return req<ProjectTokenStats>(
      `/projects/${encodeURIComponent(id)}/token-stats?${q}`,
      opts?.signal ? { signal: opts.signal } : undefined,
    )
  },
}

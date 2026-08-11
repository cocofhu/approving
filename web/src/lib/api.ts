import { reactive } from 'vue'
import type {
  Workflow,
  WorkflowVersion,
  WorkflowNotifyPolicy,
  Run,
  Artifact,
  InboxItem,
  GateShareInboxStatus,
  AcpEvent,
  WorkflowGraph,
  Project,
  ProjectAuditEvent,
  ProjectAuditFacets,
  ProjectAuditStats,
  ProjectTokenStats,
  TokenStatsWindow,
  PmLeaderBinding,
  ProjectMemoryItem,
  RequirementDraft,
  AgentCronJob,
  ChatThread,
  ChatMessage,
  PmDraftResponse,
  ProgressCitation,
  AttachedContext,
  ClarifyImage,
  ReactAnnotation,
} from './types'
import type { InboxContextResponse } from './inboxContext'
import { i18n } from './i18n'
import { authRedirectPath } from './useAuth'
import {
  isDraining,
  isMutationMethod,
  mutationsBlocked,
  showDrainToast,
  shutdownState,
} from './useShutdownState'

// API base: configurable via VITE_API_BASE, defaults to same-origin /api.
// The app is purely API-driven (no bundled mock data); on error views show
// an empty/error state.
const BASE = ((import.meta as any).env?.VITE_API_BASE ?? '/api').replace(/\/$/, '')

/** Browser URL for a stored attachment (`blob:{id}` → `/api/blobs/{id}`). */
export function blobContentUrl(ref: string): string {
  const id = String(ref || '').trim().replace(/^blob:/, '')
  if (!id) return ''
  return `${BASE}/blobs/${encodeURIComponent(id)}`
}

const AUTH_WHITELIST = new Set(['/auth/login', '/auth/logout', '/auth/me', '/health', '/live'])

function redirectToLogin() {
  if (typeof window === 'undefined') return
  const path = window.location.pathname + window.location.search
  if (path.startsWith('/login')) return
  const redirect = encodeURIComponent(path)
  window.location.assign(`/login?redirect=${redirect}`)
}

export const apiState = reactive({ online: false, checked: false })

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? 'GET').toUpperCase()
  if (mutationsBlocked() && isMutationMethod(method)) {
    const msg = shutdownState.message || i18n.global.t('common.shutdown.notAcceptingRequests')
    showDrainToast(msg)
    throw new Error(msg)
  }

  const res = await fetch(BASE + path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  })
  apiState.checked = true

  if (!res.ok) {
    if (res.status === 401 && !AUTH_WHITELIST.has(path)) {
      redirectToLogin()
      throw new Error('unauthorized')
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
        if (isMutationMethod(method)) showDrainToast(msg)
        throw new Error(msg)
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
  }
  apiState.online = true
  return (await res.json()) as T
}

export type AgentTestRepo = { name: string; url: string; branch?: string }

export type CreateAgentTestPayload = {
  repos?: AgentTestRepo[]
  repoUrl?: string
  projectId?: string
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
  hasMore: boolean
}

export interface PreviewPort {
  runId: string
  nodeId: string
  port: number
  label?: string
  proxyUrl: string
  healthy: boolean
  registeredAt?: string
}

export interface PreviewIssue {
  id: string
  runId: string
  nodeId: string
  body: string
  selector?: string
  port?: number
  images?: ClarifyImage[]
  status: string
  createdAt: string
}

export interface EventPaginatedResponse {
  events: AcpEvent[]
  nextCursor: string
  hasMore: boolean
  live?: boolean
}

export function isPaginated<T>(data: T[] | PaginatedResponse<T>): data is PaginatedResponse<T> {
  return data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data
}

export interface MCPServer {
  name: string
  url?: string
  headers?: Record<string, string>
  command?: string
  args?: string[]
  env?: Record<string, string>
}

export interface AgentFile {
  path: string
  content: string
}

export interface AgentLayout {
  configRoot?: string
  workspaceDir?: string
}

// AgentPrompts overrides the platform-injected prompt text and sandbox rule
// files for one Agent. Every field is optional; an empty field falls back to
// the platform default. The templated fields use a `{name}` placeholder for the
// declared produces file name.
export interface AgentPrompts {
  upstreamArtifactsHeader?: string
  producesContract?: string
  reactOpenSuffix?: string
  producesRetry?: string
}

export interface Agent {
  name: string
  /** Single home project; empty/undefined = unbound (artifact-store only). */
  projectId?: string
  acpBackend?: 'cursor' | 'claude_code' | 'codebuddy' | 'trae'
  gitCredentialType?: 'github_https' | 'gitlab_https' | 'ssh'
  files?: AgentFile[]
  mcp?: MCPServer[]
  env?: Record<string, string>
  layout?: AgentLayout
  prompts?: AgentPrompts
}

/** Virtual group in the Agent Studio organization tree (not a disk directory). */
export interface OrgGroup {
  id: string
  name: string
  parentGroupId?: string
}

/** Per-agent organization membership (orthogonal to skill_profile identity). */
export interface OrgAgentMembership {
  groupIds?: string[]
  parentAgent?: string
}

/** Central organization index (GET/PUT /agents/org). */
export interface AgentOrg {
  revision: number
  groups: OrgGroup[]
  agents: Record<string, OrgAgentMembership>
}

/** Result of POST /agents/org/import. */
export interface OrgFolderImportResult {
  org: AgentOrg
  created?: string[]
  overwritten?: string[]
  renamed?: Record<string, string>
}

/** Create Agent Team bootstrap progress (GET/POST /agent-teams/bootstrap). */
export interface TeamBootstrapEvent {
  kind: string
  message: string
  at: string
}

export interface TeamBootstrapResource {
  kind: string
  name: string
  detail?: string
}

export interface TeamBootstrapSession {
  id: string
  status: 'starting' | 'running' | 'ready' | 'failed' | string
  error?: string
  projectId?: string
  rootGroupId?: string
  pipelineGroupId?: string
  pmAgent?: string
  sandboxId?: string
  prefix?: string
  background?: string
  allowedGroupIds?: string[]
  agentNames?: string[]
  events: TeamBootstrapEvent[]
  resources: TeamBootstrapResource[]
  createdAt: string
  updatedAt: string
}

export interface TeamBootstrapRequest {
  projectName: string
  prefix: string
  rootGroupName: string
  pipelineGroupName: string
  pmName: string
  background: string
  acpBackend: string
  apiKey?: string
  region?: string
  gitUrl?: string
  gitCredentialType?: string
  mcp?: MCPServer[]
  env?: Record<string, string>
}

function filenameFromContentDisposition(header: string | null, fallback: string): string {
  if (!header) return fallback
  const star = /filename\*=(?:UTF-8''|utf-8'')([^;]+)/i.exec(header)
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1])
    } catch {
      /* ignore */
    }
  }
  const quoted = /filename="([^"]+)"/i.exec(header)
  if (quoted?.[1]) return quoted[1]
  const plain = /filename=([^;]+)/i.exec(header)
  if (plain?.[1]) return plain[1].trim()
  return fallback
}

export interface SandboxView {
  id: number
  name: string
  profile: string
  purpose: string
  status: string
  error?: string
  repoUrl?: string
  runId?: string
  workflowId?: string
  workflowName?: string
  nodeId?: string
  destroyAt?: string
  createdAt: string
  updatedAt: string
  containerStatus: string
  busy: boolean
  connected: boolean
  hasCodeServer: boolean
  hasAcp: boolean
  /** Same secret as container PASSWORD / CURSOR_ACP_PASSWORD for direct host:port login. */
  password?: string
  /** Gateway host:port map; only present on getSandbox (GetView), not list. */
  endpoints?: Record<string, string>
}

export interface DashboardStats {
  running: number
  waitingHuman: number
  failed: number
  completed: number
  workflows: number
  artifacts: number
  /** Platform-wide cumulative tokens; null = never reported (UI "—"). */
  totalTokens?: number | null
  workflowTokens?: number | null
  pmTokens?: number | null
}

// SettingItem is one platform scheduling knob: its effective value, where it
// came from (env|db|config) and whether it's pinned by an env var (read-only).
export interface SettingItem {
  key: string
  label: string
  unit?: string
  value: number
  min: number
  source: 'env' | 'db' | 'config'
  locked: boolean
}

export type PlatformRuleSource = 'override' | 'global' | 'embed'

export interface PlatformRuleMeta {
  file: string
  source: PlatformRuleSource
  mtime?: string
}

export interface PlatformRuleContent extends PlatformRuleMeta {
  content: string
}

export interface ChannelConfig {
  id: string
  type: string
  name: string
  enabled: boolean
  projectId: string
  appId: string
  appSecretSet: boolean
  turnTimeoutSeconds: number
  cronDeliver: boolean
  cronDeliverTarget?: string
  config?: Record<string, unknown>
  createdAt: string
  updatedAt: string
}

// Per-project channel upsert payload. type is fixed to "qq" server-side and
// projectId is implied by the request path, so neither is sent by the client.
export interface ChannelConfigInput {
  name: string
  enabled: boolean
  appId: string
  appSecret?: string
  turnTimeoutSeconds: number
  cronDeliver: boolean
  cronDeliverTarget?: string
  config?: Record<string, unknown>
}

export const api = {
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
  createRequirementDraft: (projectId: string) =>
    req<RequirementDraft>(
      `/projects/${encodeURIComponent(projectId)}/requirement-drafts`,
      { method: 'POST' },
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

  // PM Leader
  getPmLeader: (projectId: string) => req<PmLeaderBinding>(`/projects/${projectId}/pm-leader`),
  updatePmLeader: (projectId: string, body: {
    enabled?: boolean
    agentConfigRef?: string
    enabledMcps?: string[]
    gateAutoVar?: string
    gateAutoPrompt?: string
  }) =>
    req<PmLeaderBinding>(`/projects/${projectId}/pm-leader`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  listProjectCronJobs: (projectId: string) =>
    req<{ items: AgentCronJob[] }>(`/projects/${projectId}/cron-jobs`),
  patchProjectCronJob: (projectId: string, jobId: string, body: { deliverToChannel: boolean }) =>
    req<AgentCronJob>(`/projects/${projectId}/cron-jobs/${jobId}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  deleteProjectCronJob: (projectId: string, jobId: string) =>
    req<{ status: string }>(`/projects/${projectId}/cron-jobs/${jobId}`, { method: 'DELETE' }),
  listPmMemories: (projectId: string) =>
    req<{ items: ProjectMemoryItem[] }>(`/projects/${projectId}/pm/memories`),
  upsertPmMemory: (projectId: string, body: { title: string; content: string }) =>
    req<ProjectMemoryItem>(`/projects/${projectId}/pm/memories`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  updatePmMemory: (projectId: string, mid: string, body: { title?: string; content?: string }) =>
    req<ProjectMemoryItem>(`/projects/${projectId}/pm/memories/${mid}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  deletePmMemory: (projectId: string, mid: string) =>
    req<{ status: string }>(`/projects/${projectId}/pm/memories/${mid}`, { method: 'DELETE' }),
  clearPmMemories: (projectId: string) =>
    req<{ status: string; count: number }>(`/projects/${projectId}/pm/memories`, { method: 'DELETE' }),
  listPmThreads: (projectId: string) =>
    req<{ items: ChatThread[] }>(`/projects/${projectId}/pm/threads`),
  createPmThread: (projectId: string, body?: { title?: string }) =>
    req<ChatThread>(`/projects/${projectId}/pm/threads`, {
      method: 'POST',
      body: JSON.stringify(body || {}),
    }),
  getPmThread: (projectId: string, tid: string) =>
    req<ChatThread>(`/projects/${projectId}/pm/threads/${tid}`),
  deletePmThread: (projectId: string, tid: string) =>
    req<{ status: string }>(`/projects/${projectId}/pm/threads/${tid}`, { method: 'DELETE' }),
  listPmMessages: (
    projectId: string,
    tid: string,
    params?: { limit?: number; before?: string },
  ) => {
    const qs = new URLSearchParams()
    if (params?.limit != null) qs.set('limit', String(params.limit))
    if (params?.before) qs.set('before', params.before)
    const q = qs.toString()
    return req<{ items: ChatMessage[]; hasMore?: boolean }>(
      `/projects/${projectId}/pm/threads/${tid}/messages${q ? `?${q}` : ''}`,
    )
  },
  appendPmMessage: (
    projectId: string,
    tid: string,
    body: {
      role?: string
      content: string
      images?: ClarifyImage[]
      citations?: ProgressCitation[]
      attachedContext?: AttachedContext
    },
  ) =>
    req<ChatMessage>(`/projects/${projectId}/pm/threads/${tid}/messages`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  patchPmMessage: (
    projectId: string,
    tid: string,
    mid: string,
    body: { status: 'ok' | 'failed' | string; failKind?: string },
  ) =>
    req<ChatMessage>(`/projects/${projectId}/pm/threads/${tid}/messages/${mid}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  ensurePmSandbox: (
    projectId: string,
    tid: string,
    body?: { attachedContext?: AttachedContext; injectHistory?: boolean },
  ) =>
    req<{ sandbox: SandboxView; preamble?: string; thread: ChatThread }>(
      `/projects/${projectId}/pm/threads/${tid}/sandbox`,
      { method: 'POST', body: JSON.stringify(body || {}) },
    ),
  getPmDraft: (projectId: string, tid: string) =>
    req<PmDraftResponse>(`/projects/${projectId}/pm/threads/${tid}/draft`),
  /** PM turn-runner WebSocket (decoupled from sandbox request ctx). */
  pmThreadChatWsUrl: (projectId: string, tid: string) =>
    wsUrl(`/projects/${projectId}/pm/threads/${tid}/chat`),

  // workflows
  listWorkflows: (params?: { projectId?: string }) => {
    const qs = new URLSearchParams()
    if (params?.projectId) qs.set('projectId', params.projectId)
    const q = qs.toString()
    return req<Workflow[]>(q ? `/workflows?${q}` : '/workflows')
  },
  getWorkflow: (id: string, opts?: { signal?: AbortSignal }) =>
    req<Workflow>(`/workflows/${id}`, opts?.signal ? { signal: opts.signal } : undefined),
  saveWorkflow: (wf: Partial<Workflow>) =>
    req<Workflow>(wf.id ? `/workflows/${wf.id}` : '/workflows', {
      method: wf.id ? 'PUT' : 'POST',
      body: JSON.stringify(wf),
    }),
  /** Notify-only: never sends nodes/edges (avoids stale list-cache graph rollback). */
  patchWorkflowNotifyPolicy: (id: string, notifyPolicy: WorkflowNotifyPolicy) =>
    req<Workflow>(`/workflows/${id}/notify-policy`, {
      method: 'PATCH',
      body: JSON.stringify({ notifyPolicy }),
    }),
  publishWorkflow: (id: string) => req<Workflow>(`/workflows/${id}/publish`, { method: 'POST' }),
  listAPIKeys: (workflowId: string) =>
    req<{ id: string; name: string; key_prefix: string; created_at: string }[]>(`/workflows/${workflowId}/api-keys`),
  createAPIKey: (workflowId: string, name: string) =>
    req<{ id: string; name: string; key: string; key_prefix: string; created_at: string }>(`/workflows/${workflowId}/api-keys`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  revokeAPIKey: (workflowId: string, keyId: string) =>
    req<{ status: string }>(`/workflows/${workflowId}/api-keys/${keyId}`, { method: 'DELETE' }),
  deleteWorkflow: (id: string) => req<{ status: string }>(`/workflows/${id}`, { method: 'DELETE' }),
  listWorkflowVersions: (id: string, opts?: { signal?: AbortSignal }) =>
    req<WorkflowVersion[]>(`/workflows/${id}/versions`, opts?.signal ? { signal: opts.signal } : undefined),
  restoreWorkflowVersion: (id: string, version: number) =>
    req<Workflow>(`/workflows/${id}/versions/${version}/restore`, { method: 'POST' }),
  copyPreviewWorkflow: (id: string) =>
    req<{ suggestedName: string; sourceName: string; sourceId: string }>(`/workflows/${id}/copy-preview`),
  copyWorkflow: (id: string, name: string, opts?: { signal?: AbortSignal }) =>
    req<Workflow>(`/workflows/${id}/copy`, {
      method: 'POST',
      body: JSON.stringify({ name }),
      ...(opts?.signal ? { signal: opts.signal } : {}),
    }),
  getWorkflowVersionGraph: (id: string, version: number, opts?: { signal?: AbortSignal }) =>
    req<WorkflowGraph>(
      `/workflows/${id}/versions/${version}/graph`,
      opts?.signal ? { signal: opts.signal } : undefined,
    ),
  importWorkflow: (json: string, projectId?: string) => {
    const qs = projectId ? `?projectId=${encodeURIComponent(projectId)}` : ''
    return req<Workflow>(`/workflows/import${qs}`, { method: 'POST', body: json })
  },

  // runs
  listRuns: (params?: {
    status?: string
    tag?: string
    wf?: string
    projectId?: string
    page?: number
    pageSize?: number
    /** Whitelist: started_at | priority. Only sent when paired with a valid order. */
    sort?: string
    /** Whitelist: asc | desc. Only sent when paired with a valid sort. */
    order?: 'asc' | 'desc' | string
    signal?: AbortSignal
  }) => {
    const qs = new URLSearchParams()
    if (params?.status) qs.set('status', params.status)
    if (params?.tag) qs.set('tag', params.tag)
    if (params?.wf) qs.set('wf', params.wf)
    if (params?.projectId) qs.set('projectId', params.projectId)
    if (params?.page != null) qs.set('page', String(params.page))
    if (params?.pageSize != null) qs.set('pageSize', String(params.pageSize))
    const sort = params?.sort
    const order = params?.order
    if (
      (sort === 'started_at' || sort === 'priority') &&
      (order === 'asc' || order === 'desc')
    ) {
      qs.set('sort', sort)
      qs.set('order', order)
    }
    const q = qs.toString()
    const path = q ? `/runs?${q}` : '/runs'
    const init = params?.signal ? { signal: params.signal } : undefined
    if (params?.page != null || params?.pageSize != null) {
      return req<PaginatedResponse<Run>>(path, init)
    }
    return req<Run[]>(path, init)
  },
  getRun: (id: string) => req<Run>(`/runs/${id}`),
  inboxContext: (
    runId: string,
    nodeId: string,
    iteration: number,
    opts?: { signal?: AbortSignal },
  ) =>
    req<InboxContextResponse>(
      `/runs/${runId}/inbox-context?nodeId=${encodeURIComponent(nodeId)}&iteration=${iteration}`,
      opts?.signal ? { signal: opts.signal } : undefined,
    ),
  listProjectRunTags: (projectId: string) =>
    req<{ tags: string[] }>(`/projects/${encodeURIComponent(projectId)}/run-tags`),
  startRun: (
    workflowId: string,
    inputs: Record<string, any>,
    trigger = 'manual',
    priority = 'normal',
    tags: string[] = [],
    opts?: { signal?: AbortSignal },
  ) =>
    req<{ id: string; status: string; priority?: string }>(`/workflows/${workflowId}/runs`, {
      method: 'POST',
      body: JSON.stringify({ inputs, trigger, priority, tags }),
      ...(opts?.signal ? { signal: opts.signal } : {}),
    }),
  updateRunPriority: (id: string, priority: string) =>
    req<{ id: string; status: string; priority: string }>(`/runs/${id}/priority`, {
      method: 'PATCH',
      body: JSON.stringify({ priority }),
    }),
  cancelRun: (id: string) => req<{ status: string }>(`/runs/${id}/cancel`, { method: 'POST' }),
  // Hard-delete a completed/failed run. Missing → 404; non-deletable status → 409.
  deleteRun: (id: string) => req<{ status: string }>(`/runs/${id}`, { method: 'DELETE' }),
  resumeRun: (id: string, nodeId = '') =>
    req<{ status: string }>(`/runs/${id}/resume`, {
      method: 'POST',
      body: JSON.stringify({ nodeId }),
    }),
  runEventsWsUrl: (id: string) => wsUrl(`/runs/${id}/events`),
  resumeGate: (runId: string, nodeId: string, action: string, form: Record<string, any> = {}) =>
    req<{ status: string }>(`/runs/${runId}/gates/${nodeId}/resume`, {
      method: 'POST',
      body: JSON.stringify({ action, form }),
    }),
  createGateShareLink: (runId: string, nodeId: string, ttlTier = '24h') =>
    req<{ id: string; url: string; ttlTier: string; expiresAt: string; state: string }>(
      `/runs/${runId}/gates/${nodeId}/share-link`,
      { method: 'POST', body: JSON.stringify({ ttlTier }) },
    ),
  getGateShareLink: (runId: string, nodeId: string) =>
    req<GateShareInboxStatus>(`/runs/${runId}/gates/${nodeId}/share-link`),
  regenGateShareLink: (runId: string, nodeId: string) =>
    req<{ id: string; url: string; ttlTier: string; expiresAt: string; state: string }>(
      `/runs/${runId}/gates/${nodeId}/share-link/regen`,
      { method: 'POST' },
    ),
  revokeGateShareLink: (runId: string, nodeId: string) =>
    req<{ status: string }>(`/runs/${runId}/gates/${nodeId}/share-link/revoke`, { method: 'POST' }),
  createReviewShareLink: (runId: string, nodeId: string, ttlTier = '24h') =>
    req<{ id: string; url: string; ttlTier: string; expiresAt: string; state: string }>(
      `/runs/${runId}/reviews/${nodeId}/share-link`,
      { method: 'POST', body: JSON.stringify({ ttlTier }) },
    ),
  getReviewShareLink: (runId: string, nodeId: string) =>
    req<GateShareInboxStatus>(`/runs/${runId}/reviews/${nodeId}/share-link`),
  regenReviewShareLink: (runId: string, nodeId: string) =>
    req<{ id: string; url: string; ttlTier: string; expiresAt: string; state: string }>(
      `/runs/${runId}/reviews/${nodeId}/share-link/regen`,
      { method: 'POST' },
    ),
  revokeReviewShareLink: (runId: string, nodeId: string) =>
    req<{ status: string }>(`/runs/${runId}/reviews/${nodeId}/share-link/revoke`, { method: 'POST' }),
  listGatePrimaryArtifacts: (runId: string, nodeId: string) =>
    req<{
      items: {
        name: string
        kind: string
        readonly?: boolean
        nodeId?: string
        outputKey?: string
      }[]
    }>(`/runs/${runId}/gates/${nodeId}/primary-artifacts`),
  saveGateArtifact: (
    runId: string,
    nodeId: string,
    name: string,
    content: string,
    ifMatch?: string,
  ) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (ifMatch) headers['If-Match'] = ifMatch
    return req<{
      id: string
      name: string
      kind: string
      sizeBytes: number
      updatedAt: string
      etag: string
      nodeId: string
      content: string
    }>(`/runs/${runId}/gates/${nodeId}/artifacts/${encodeURIComponent(name)}`, {
      method: 'PUT',
      headers,
      body: JSON.stringify({ content }),
    })
  },
  reactReply: (
    runId: string,
    nodeId: string,
    text: string,
    images: ClarifyImage[] = [],
    force = false,
    annotations: ReactAnnotation[] = [],
  ) =>
    req<{ status: string; waiting?: number }>(`/runs/${runId}/react/${nodeId}/reply`, {
      method: 'POST',
      body: JSON.stringify({ text, images, force, annotations }),
    }),
  /** 轮级 Cancel for node-inline review (clears FIFO + aborts active ACP turn). */
  reactCancel: (runId: string, nodeId: string) =>
    req<{ status: string }>(`/runs/${runId}/react/${nodeId}/cancel`, { method: 'POST' }),
  // Approval-gate ReAct reject: send annotations/text/images to the gate's
  // upstream producer's still-alive session for an in-place edit; the gate stays
  // pending. Requires gate.reactSessionAlive.
  gateReactRevise: (
    runId: string,
    nodeId: string,
    text: string,
    images: ClarifyImage[] = [],
    annotations: ReactAnnotation[] = [],
  ) =>
    req<{ status: string; waiting?: number; producerNodeId?: string }>(
      `/runs/${runId}/gates/${nodeId}/react-revise`,
      {
        method: 'POST',
        body: JSON.stringify({ text, images, annotations }),
      },
    ),
  /** 轮级 Cancel for gate hot-revise (upstream producer session). */
  gateReactCancel: (runId: string, nodeId: string) =>
    req<{ status: string; producerNodeId?: string }>(
      `/runs/${runId}/gates/${nodeId}/react-cancel`,
      { method: 'POST' },
    ),

  // agents (reusable, user-defined Agent identities referenced by skill_profile:
  // skill/rules + MCP servers + environment variables)
  listAgents: () => req<Agent[]>('/agents'),
  bootstrapProjectOnboarding: (
    projectId: string,
    body: {
      acpBackend: string
      apiKey: string
      region?: string
      repos?: string
      featureHint?: string
    },
  ) =>
    req<{
      agentIds: string[]
      workflowId: string
      repos: string
      feature: string
      published: boolean
    }>(`/projects/${encodeURIComponent(projectId)}/bootstrap-onboarding`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  bootstrapAgentTeam: (body: TeamBootstrapRequest) =>
    req<TeamBootstrapSession>('/agent-teams/bootstrap', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  getAgentTeamBootstrap: (id: string) =>
    req<TeamBootstrapSession>(`/agent-teams/bootstrap/${encodeURIComponent(id)}`),
  retryAgentTeamBootstrap: (id: string) =>
    req<TeamBootstrapSession>(`/agent-teams/bootstrap/${encodeURIComponent(id)}/retry`, {
      method: 'POST',
    }),
  listAgentTeamTemplates: () =>
    req<{ items: { id: string; embedName: string; roleLabelZh: string; summary: string }[] }>(
      '/agent-teams/templates',
    ),
  getAgentsOrg: () => req<AgentOrg>('/agents/org'),
  saveAgentsOrg: (org: AgentOrg) =>
    req<AgentOrg>('/agents/org', { method: 'PUT', body: JSON.stringify(org) }),
  createAgent: (agent: Agent) =>
    req<Agent>('/agents', { method: 'POST', body: JSON.stringify(agent) }),
  saveAgent: (agent: Agent) =>
    req<{ status: string }>(`/agents/${encodeURIComponent(agent.name)}`, {
      method: 'PUT',
      body: JSON.stringify(agent),
    }),
  /** Group-level assign: only changes projectId; does not rewrite workspace. Unbind is not allowed. */
  patchAgentProject: (name: string, projectId: string) =>
    req<{ status: string; projectId: string }>(`/agents/${encodeURIComponent(name)}/project`, {
      method: 'PATCH',
      body: JSON.stringify({ projectId }),
    }),
  renameAgent: (name: string, newName: string) =>
    req<Agent & { updatedWorkflowCount?: number }>(`/agents/${encodeURIComponent(name)}/rename`, {
      method: 'POST',
      body: JSON.stringify({ name: newName }),
    }),
  deleteAgent: (name: string) =>
    req<{ status: string }>(`/agents/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // Agent-scoped data (Studio). Project resolved server-side from agent.projectId.
  listAgentMemories: (name: string) =>
    req<{ items: ProjectMemoryItem[] }>(`/agents/${encodeURIComponent(name)}/memories`),
  upsertAgentMemory: (name: string, body: { title: string; content: string }) =>
    req<ProjectMemoryItem>(`/agents/${encodeURIComponent(name)}/memories`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  updateAgentMemory: (name: string, mid: string, body: { title?: string; content?: string }) =>
    req<ProjectMemoryItem>(`/agents/${encodeURIComponent(name)}/memories/${mid}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  deleteAgentMemory: (name: string, mid: string) =>
    req<{ status: string }>(`/agents/${encodeURIComponent(name)}/memories/${mid}`, { method: 'DELETE' }),
  clearAgentMemories: (name: string) =>
    req<{ status: string; count: number }>(`/agents/${encodeURIComponent(name)}/memories`, { method: 'DELETE' }),
  listAgentThreads: (name: string) =>
    req<{ items: ChatThread[]; messageCounts: Record<string, number> }>(
      `/agents/${encodeURIComponent(name)}/threads`,
    ),
  listAgentThreadMessages: (name: string, tid: string) =>
    req<{ items: ChatMessage[]; total: number }>(
      `/agents/${encodeURIComponent(name)}/threads/${tid}/messages`,
    ),
  deleteAgentThread: (name: string, tid: string) =>
    req<{ status: string }>(`/agents/${encodeURIComponent(name)}/threads/${tid}`, { method: 'DELETE' }),
  listAgentCronJobs: (name: string) =>
    req<{ items: AgentCronJob[] }>(`/agents/${encodeURIComponent(name)}/cron-jobs`),
  patchAgentCronJob: (name: string, jobId: string, body: { enabled?: boolean; deliverToChannel?: boolean }) =>
    req<AgentCronJob>(`/agents/${encodeURIComponent(name)}/cron-jobs/${jobId}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  deleteAgentCronJob: (name: string, jobId: string) =>
    req<{ status: string }>(`/agents/${encodeURIComponent(name)}/cron-jobs/${jobId}`, { method: 'DELETE' }),

  exportAgent: async (name: string): Promise<Blob> => {
    const res = await fetch(`${BASE}/agents/${encodeURIComponent(name)}/export`, {
      credentials: 'include',
    })
    if (!res.ok) {
      let msg = `${res.status} export failed`
      try {
        const body = await res.json()
        if (body?.error) msg = body.error
      } catch {
        // non-JSON
      }
      throw new Error(msg)
    }
    return res.blob()
  },
  exportOrgFolder: async (groupId: string): Promise<{ blob: Blob; filename: string }> => {
    const res = await fetch(`${BASE}/agents/org/export?groupId=${encodeURIComponent(groupId)}`, {
      credentials: 'include',
    })
    if (!res.ok) {
      let msg = `${res.status} export failed`
      try {
        const body = await res.json()
        if (body?.error) msg = body.error
      } catch {
        // non-JSON
      }
      throw new Error(msg)
    }
    const blob = await res.blob()
    const filename = filenameFromContentDisposition(
      res.headers.get('Content-Disposition'),
      'folder.zip',
    )
    return { blob, filename }
  },
  importOrgFolder: async (
    zipFile: File,
    opts: { targetGroupId?: string; mode: 'rename' | 'overwrite' },
  ): Promise<OrgFolderImportResult> => {
    const fd = new FormData()
    fd.append('file', zipFile)
    if (opts.targetGroupId) fd.append('targetGroupId', opts.targetGroupId)
    fd.append('mode', opts.mode)
    const res = await fetch(`${BASE}/agents/org/import`, {
      method: 'POST',
      credentials: 'include',
      body: fd,
    })
    if (!res.ok) {
      let msg = `${res.status} import failed`
      try {
        const body = await res.json()
        if (body?.error) msg = body.error
      } catch {
        // non-JSON
      }
      throw new Error(msg)
    }
    return (await res.json()) as OrgFolderImportResult
  },
  importAgent: async (zipFile: File, opts: { targetName: string; mode: 'create' | 'overwrite' }): Promise<Agent> => {
    const fd = new FormData()
    fd.append('file', zipFile)
    fd.append('targetName', opts.targetName)
    fd.append('mode', opts.mode)
    const res = await fetch(`${BASE}/agents/import`, {
      method: 'POST',
      credentials: 'include',
      body: fd,
    })
    if (!res.ok) {
      let msg = `${res.status} import failed`
      try {
        const body = await res.json()
        if (body?.error) msg = body.error
      } catch {
        // non-JSON
      }
      throw new Error(msg)
    }
    return (await res.json()) as Agent
  },

  // sandboxes (interactive Agent chat-test containers)
  createAgentTest: (profile: string, payload: CreateAgentTestPayload = {}) =>
    req<SandboxView>(`/agents/${encodeURIComponent(profile)}/test`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  listSandboxes: () => req<SandboxView[]>('/sandboxes'),
  getSandbox: (id: number) => req<SandboxView>(`/sandboxes/${id}`),
  stopSandbox: (id: number) => req<{ status: string }>(`/sandboxes/${id}/stop`, { method: 'POST' }),
  destroySandbox: (id: number) => req<{ status: string }>(`/sandboxes/${id}`, { method: 'DELETE' }),
  cleanupSandboxes: () => req<{ destroyed: number; skipped: number }>('/sandboxes/cleanup', { method: 'POST' }),
  sandboxChatWsUrl: (id: number) => wsUrl(`/sandboxes/${id}/chat`),
  sandboxTerminalWsUrl: (id: number) => wsUrl(`/sandboxes/${id}/terminal`),
  sandboxIdeUrl: (id: number) => `${origin()}/sandbox/${id}/?folder=/root/workspace`,
  // Reverse-proxy to the in-container acp-bridge native web UI (8765). The
  // trailing slash matters so the UI resolves its relative assets/WS against
  // this subpath (document.baseURI).
  sandboxBridgeUrl: (id: number) => `${origin()}/sandbox-bridge/${id}/`,
  /** @deprecated Use sandboxBridgeUrl. Remove in 0.2.0. */
  sandboxAcpUrl: (id: number) => `${origin()}/sandbox-bridge/${id}/`,

  // misc
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
  exportRunLogsUrl: (id: string) => `${origin()}/api/runs/${id}/logs/export`,
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
  // Agent event log, read straight from the node's live sandbox (falls back to
  // the persisted snapshot once the sandbox is gone).
  nodeEvents: (
    runId: string,
    nodeId: string,
    params?: { cursor?: string; limit?: number; signal?: AbortSignal },
  ) => {
    const qs = new URLSearchParams()
    if (params?.cursor) qs.set('cursor', params.cursor)
    if (params?.limit != null) qs.set('limit', String(params.limit))
    const q = qs.toString()
    const path = `/runs/${runId}/nodes/${nodeId}/events` + (q ? `?${q}` : '')
    const init = params?.signal ? { signal: params.signal } : undefined
    if (params?.cursor || params?.limit != null) {
      return req<EventPaginatedResponse>(path, init)
    }
    return req<{ events: AcpEvent[]; live: boolean }>(path, init)
  },
  // Raw agent event frames (unaggregated) — used to rebuild the chat transcript
  // when reopening a reused sandbox.
  sandboxEventLog: (id: number, params?: { cursor?: string; limit?: number }) => {
    const qs = new URLSearchParams()
    if (params?.cursor) qs.set('cursor', params.cursor)
    if (params?.limit != null) qs.set('limit', String(params.limit))
    const q = qs.toString()
    const path = `/sandboxes/${id}/eventlog` + (q ? `?${q}` : '')
    if (params?.cursor || params?.limit != null) {
      return req<{ events: any[]; nextCursor: string; hasMore: boolean }>(path)
    }
    return req<{ events: any[] }>(path)
  },
  // Raw sandbox container logs (docker logs): live if running, else the archived
  // snapshot captured at teardown. Used for post-mortem troubleshooting.
  // `error` is set when the control plane should have logs but the read failed
  // (distinct from found=false = no log source).
  nodeSandboxLog: (runId: string, nodeId: string, opts?: { signal?: AbortSignal }) =>
    req<{ content: string; live: boolean; found: boolean; error?: string }>(
      `/runs/${runId}/nodes/${nodeId}/sandbox-log`,
      opts?.signal ? { signal: opts.signal } : undefined,
    ),
  getRunNodeSandbox: async (runId: string, nodeId: string): Promise<SandboxView | null> => {
    const res = await fetch(BASE + `/runs/${runId}/nodes/${nodeId}/sandbox`, {
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
    })
    if (res.status === 404) return null
    if (!res.ok) {
      let msg = `${res.status} /runs/${runId}/nodes/${nodeId}/sandbox`
      try {
        const body = await res.json()
        if (body?.error) msg = body.error
      } catch {
        // non-JSON error body
      }
      throw new Error(msg)
    }
    apiState.checked = true
    apiState.online = true
    return (await res.json()) as SandboxView
  },
  sandboxLog: (id: number, opts?: { signal?: AbortSignal }) =>
    req<{ content: string; live: boolean; found: boolean }>(
      `/sandboxes/${id}/log`,
      opts?.signal ? { signal: opts.signal } : undefined,
    ),
  nodePreviews: (runId: string, nodeId: string, opts?: { signal?: AbortSignal }) =>
    req<{ ports: PreviewPort[] }>(
      `/runs/${runId}/nodes/${nodeId}/previews`,
      opts?.signal ? { signal: opts.signal } : undefined,
    ),
  listPreviewIssues: (runId: string, nodeId: string) =>
    req<{ issues: PreviewIssue[] }>(`/runs/${runId}/nodes/${nodeId}/preview-issues`),
  createPreviewIssue: (
    runId: string,
    nodeId: string,
    body: string,
    selector = '',
    port = 0,
    images: ClarifyImage[] = [],
  ) =>
    req<PreviewIssue>(`/runs/${runId}/nodes/${nodeId}/preview-issues`, {
      method: 'POST',
      body: JSON.stringify({ body, selector, port, images }),
    }),
  deletePreviewIssue: (runId: string, nodeId: string, issueId: string) =>
    req<{ status: string }>(`/runs/${runId}/nodes/${nodeId}/preview-issues/${issueId}`, {
      method: 'DELETE',
    }),
  health: () =>
    req<{ status: string; ready: boolean; vnc_preview?: boolean }>(`/health`),
  previewVncWsUrl: (runId: string, nodeId: string, port: number) =>
    rootWsUrl(`/preview-vnc/${runId}/${nodeId}/${port}/ws`),
  /** Console noVNC: sandbox-scoped WS (not preview runId/nodeId/port). */
  sandboxVncWsUrl: (sandboxId: number) =>
    rootWsUrl(`/sandbox-vnc/${sandboxId}/ws`),
  listGates: (params?: {
    page?: number
    pageSize?: number
    wf?: string
    projectId?: string
    tag?: string
    signal?: AbortSignal
  }) => {
    const qs = new URLSearchParams()
    if (params?.page != null) qs.set('page', String(params.page))
    if (params?.pageSize != null) qs.set('pageSize', String(params.pageSize))
    if (params?.wf) qs.set('wf', params.wf)
    if (params?.projectId) qs.set('projectId', params.projectId)
    if (params?.tag) qs.set('tag', params.tag)
    const q = qs.toString()
    const path = q ? `/gates?${q}` : '/gates'
    const init = params?.signal ? { signal: params.signal } : undefined
    if (params?.page != null || params?.pageSize != null) {
      return req<PaginatedResponse<InboxItem>>(path, init)
    }
    return req<InboxItem[]>(path, init)
  },
  dashboard: () => req<DashboardStats>('/stats/dashboard'),
  // platform settings (scheduling params)
  getSettings: () => req<{ items: SettingItem[] }>('/settings'),
  updateSettings: (patch: Record<string, number>) =>
    req<{ items: SettingItem[] }>('/settings', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),

  listPlatformRules: () => req<{ items: PlatformRuleMeta[] }>('/platform-rules'),
  getPlatformRule: (file: string) => req<PlatformRuleContent>(`/platform-rules/${encodeURIComponent(file)}`),
  savePlatformRule: (file: string, content: string) =>
    req<PlatformRuleContent>(`/platform-rules/${encodeURIComponent(file)}`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    }),
  resetPlatformRule: (file: string) =>
    req<PlatformRuleContent>(`/platform-rules/${encodeURIComponent(file)}/reset`, { method: 'POST' }),
  getPlatformRuleEmbed: (file: string) =>
    req<PlatformRuleContent>(`/platform-rules/${encodeURIComponent(file)}/embed`),

  listAgentPlatformRules: (agent: string) =>
    req<{ items: PlatformRuleMeta[] }>(`/agents/${encodeURIComponent(agent)}/platform-rules`),
  getAgentPlatformRule: (agent: string, file: string) =>
    req<PlatformRuleContent>(`/agents/${encodeURIComponent(agent)}/platform-rules/${encodeURIComponent(file)}`),
  saveAgentPlatformRule: (agent: string, file: string, content: string) =>
    req<PlatformRuleContent>(`/agents/${encodeURIComponent(agent)}/platform-rules/${encodeURIComponent(file)}`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    }),
  deleteAgentPlatformRule: (agent: string, file: string) =>
    req<{ status: string }>(`/agents/${encodeURIComponent(agent)}/platform-rules/${encodeURIComponent(file)}`, {
      method: 'DELETE',
    }),

  // per-project external IM channel (one QQ channel per project)
  getProjectChannel: (projectId: string) =>
    req<{ channel: ChannelConfig | null; secretsKeyConfigured?: boolean }>(
      `/projects/${encodeURIComponent(projectId)}/channel`,
    ),
  putProjectChannel: (projectId: string, body: ChannelConfigInput) =>
    req<ChannelConfig>(`/projects/${encodeURIComponent(projectId)}/channel`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  deleteProjectChannel: (projectId: string) =>
    req<{ status: string }>(`/projects/${encodeURIComponent(projectId)}/channel`, { method: 'DELETE' }),
}

export interface AuthMeResponse {
  username: string
  expires_at: string
  is_admin?: boolean
}

export interface AuthLoginResponse {
  username: string
  expires_at: string
  redirect?: string
}

export const authApi = {
  login: (username: string, password: string, redirect = '/') =>
    req<AuthLoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password, redirect: authRedirectPath(redirect) }),
    }),
  logout: () => req<{ status: string }>('/auth/logout', { method: 'POST' }),
  me: () => req<AuthMeResponse>('/auth/me'),
}

// origin resolves the API origin (absolute VITE_API_BASE, else same-origin).
function origin(): string {
  if (/^https?:\/\//.test(BASE)) return BASE.replace(/\/api$/, '')
  return window.location.origin
}

// wsUrl builds a ws(s):// URL for an API path under BASE.
function wsUrl(path: string): string {
  let base = BASE
  if (!/^https?:\/\//.test(base)) base = window.location.origin + base
  return base.replace(/^http/, 'ws') + path
}

// rootWsUrl builds a ws(s):// URL for a site-root path (not under the /api base).
function rootWsUrl(path: string): string {
  return window.location.origin.replace(/^http/, 'ws') + path
}

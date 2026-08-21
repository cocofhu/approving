import type { Workflow, WorkflowVersion, WorkflowNotifyPolicy, WorkflowGraph } from '../shared/types'
import { req } from './httpCore'

export const workflowsClient = {
  // workflows
  listWorkflows: (params?: { projectId?: string; signal?: AbortSignal }) => {
    const qs = new URLSearchParams()
    if (params?.projectId) qs.set('projectId', params.projectId)
    const q = qs.toString()
    return req<Workflow[]>(q ? `/workflows?${q}` : '/workflows', params?.signal ? { signal: params.signal } : undefined)
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
}

export { apiState, blobContentUrl } from './httpCore'
export type * from './apiTypes'

import type { PaginatedResponse } from './apiTypes'
import { authRedirectPath } from '../composables/useAuth'
import { req } from './httpCore'
import type { AuthLoginResponse, AuthMeResponse } from './apiTypes'
import { projectsClient } from './projectsClient'
import { pmClient } from './pmClient'
import { workflowsClient } from './workflowsClient'
import { runsClient } from './runsClient'
import { agentsClient } from './agentsClient'
import { sandboxesClient } from './sandboxesClient'
import { artifactsClient } from './artifactsClient'
import { settingsClient } from './settingsClient'

export function isPaginated<T>(data: T[] | PaginatedResponse<T>): data is PaginatedResponse<T> {
  return data != null && typeof data === 'object' && !Array.isArray(data) && 'items' in data
}

export const api = {
  ...projectsClient,
  ...pmClient,
  ...workflowsClient,
  ...runsClient,
  ...agentsClient,
  ...sandboxesClient,
  ...artifactsClient,
  ...settingsClient,
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

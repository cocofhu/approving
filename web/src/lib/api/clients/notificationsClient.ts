import { req } from '../httpCore'
import type { NotificationListResponse } from '../apiTypes'

export type NotificationReadFilter = 'all' | 'unread' | 'read'

export const notificationsClient = {
  listNotifications: (opts?: {
    signal?: AbortSignal
    page?: number
    pageSize?: number
    filter?: NotificationReadFilter
  }) => {
    const params = new URLSearchParams()
    if (opts?.page != null && opts.page > 0) params.set('page', String(opts.page))
    if (opts?.pageSize != null && opts.pageSize > 0) params.set('pageSize', String(opts.pageSize))
    if (opts?.filter) params.set('filter', opts.filter)
    const qs = params.toString()
    return req<NotificationListResponse>(
      `/notifications${qs ? `?${qs}` : ''}`,
      opts?.signal ? { signal: opts.signal } : undefined,
    )
  },
  markNotificationRead: (runId: string) =>
    req<{ status: string }>('/notifications/read', {
      method: 'POST',
      body: JSON.stringify({ runId }),
    }),
  markAllNotificationsRead: () =>
    req<{ status: string }>('/notifications/read-all', {
      method: 'POST',
    }),
}

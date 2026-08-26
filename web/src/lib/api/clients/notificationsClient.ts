import { req } from '../httpCore'
import type { NotificationListResponse } from '../apiTypes'

export const notificationsClient = {
  listNotifications: (opts?: { signal?: AbortSignal }) =>
    req<NotificationListResponse>(
      '/notifications',
      opts?.signal ? { signal: opts.signal } : undefined,
    ),
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

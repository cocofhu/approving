import { req } from '../httpCore'
import type { NotificationReadPrefs } from '../apiTypes'

export const notificationsClient = {
  getNotificationReadPrefs: (opts?: { signal?: AbortSignal }) =>
    req<NotificationReadPrefs>('/notifications/prefs', opts?.signal ? { signal: opts.signal } : undefined),
  markNotificationRead: (runId: string) =>
    req<NotificationReadPrefs>('/notifications/prefs/read', {
      method: 'POST',
      body: JSON.stringify({ runId }),
    }),
  markAllNotificationsRead: (runIds: string[]) =>
    req<NotificationReadPrefs>('/notifications/prefs/read-all', {
      method: 'POST',
      body: JSON.stringify({ runIds }),
    }),
}

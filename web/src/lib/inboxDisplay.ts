import { truncateText } from '@/lib/format'
import type { InboxItem } from '@/lib/types'

/** Secondary-line run label: non-empty runTitle (truncated) or `Run #id`. */
export function inboxRunLabel(it: Pick<InboxItem, 'runId' | 'runTitle'>): string {
  const title = it.runTitle?.trim()
  if (title) return truncateText(title, 60)
  return `Run #${it.runId.replace(/^run-/, '')}`
}

export function inboxSecondaryLine(it: Pick<InboxItem, 'workflowName' | 'runId' | 'runTitle'>): string {
  return `${it.workflowName} · ${inboxRunLabel(it)}`
}

/** i18n key for the list badge: gate / clarify / review. */
export type InboxBadgeLabelKey =
  | 'pages.gatesInbox.gateType'
  | 'pages.gatesInbox.clarifyType'
  | 'pages.gatesInbox.reviewType'

/**
 * List badge copy key by inbox semantics.
 * gate → gateType; clarify channel with kind=review → reviewType; else clarifyType.
 */
export function inboxBadgeLabelKey(it: Pick<InboxItem, 'type'> & { kind?: string }): InboxBadgeLabelKey {
  if (it.type === 'gate') return 'pages.gatesInbox.gateType'
  if (it.kind === 'review') return 'pages.gatesInbox.reviewType'
  return 'pages.gatesInbox.clarifyType'
}

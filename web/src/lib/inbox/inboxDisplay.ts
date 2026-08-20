import { truncateText } from '@/lib/shared/format'
import type { InboxItem } from '@/lib/shared/types'

/** Secondary-line run label: non-empty runTitle (truncated) or `Run #id`. */
export function inboxRunLabel(it: Pick<InboxItem, 'runId' | 'runTitle'>): string {
  const title = it.runTitle?.trim()
  if (title) return truncateText(title, 60)
  return `Run #${it.runId.replace(/^run-/, '')}`
}

export function inboxSecondaryLine(it: Pick<InboxItem, 'workflowName' | 'runId' | 'runTitle'>): string {
  return `${it.workflowName} · ${inboxRunLabel(it)}`
}

/** i18n key for the list badge: gate / clarify / review / app_preview. */
export type InboxBadgeLabelKey =
  | 'pages.gatesInbox.gateType'
  | 'pages.gatesInbox.clarifyType'
  | 'pages.gatesInbox.reviewType'
  | 'pages.gatesInbox.previewType'
  | 'pages.gatesInbox.startingType'

/** Visual tone for icon/badge chips (Demo: warn / preview-blue / review-green / clarify-cyan). */
export type InboxBadgeTone = 'gate' | 'preview' | 'review' | 'clarify'

/** True while the item's sandbox is still booting (no transcript, no reply yet). */
export function isStartingInboxItem(
  it: Pick<InboxItem, 'type'> & { state?: string } | null | undefined,
): boolean {
  return !!it && it.type === 'clarify' && it.state === 'starting'
}

/**
 * List badge copy key by inbox semantics.
 * state=starting → startingType; gate → gateType; kind=app_preview → previewType;
 * kind=review → reviewType; else clarifyType.
 */
export function inboxBadgeLabelKey(
  it: Pick<InboxItem, 'type'> & { kind?: string; state?: string },
): InboxBadgeLabelKey {
  if (isStartingInboxItem(it)) return 'pages.gatesInbox.startingType'
  if (it.type === 'gate') return 'pages.gatesInbox.gateType'
  if (it.kind === 'app_preview') return 'pages.gatesInbox.previewType'
  if (it.kind === 'review') return 'pages.gatesInbox.reviewType'
  return 'pages.gatesInbox.clarifyType'
}

/** Badge/icon color tone; app_preview uses info blue (Demo --c-preview). */
export function inboxBadgeTone(it: Pick<InboxItem, 'type'> & { kind?: string }): InboxBadgeTone {
  if (it.type === 'gate') return 'gate'
  if (it.kind === 'app_preview') return 'preview'
  if (it.kind === 'review') return 'review'
  return 'clarify'
}

/** Icon tile classes by tone. */
export function inboxIconToneClass(tone: InboxBadgeTone): string {
  switch (tone) {
    case 'gate':
      return 'bg-warn/15 text-warn'
    case 'preview':
      return 'bg-info/15 text-info'
    case 'review':
      return 'bg-n-review/15 text-n-review'
    default:
      return 'bg-n-clarify/15 text-n-clarify'
  }
}

/** Badge chip classes by tone. */
export function inboxBadgeToneClass(tone: InboxBadgeTone): string {
  switch (tone) {
    case 'gate':
      return 'border-warn/30 bg-warn/10 text-warn'
    case 'preview':
      return 'border-info/30 bg-info/10 text-info'
    case 'review':
      return 'border-n-review/30 bg-n-review/10 text-n-review'
    default:
      return 'border-n-clarify/30 bg-n-clarify/10 text-n-clarify'
  }
}

import type { ClarifyImage } from '../shared/types'

/** localStorage key for the home Dashboard composer draft (independent of runDraft / pipeline memory). */
export const HOME_COMPOSER_DRAFT_KEY = 'approving.home.composerDraft'

export const HOME_COMPOSER_DRAFT_SCHEMA = '1'

export interface HomeDraftAttachment {
  name?: string
  mimeType: string
  /** base64 payload without data URL prefix */
  data: string
}

export interface HomeComposerDraft {
  schemaVersion: string
  savedAt: number
  pipelineId: string
  text: string
  attachments: HomeDraftAttachment[]
}

export type SaveHomeComposerDraftResult = 'ok' | 'quota_exceeded' | 'error'

function isAttachment(v: unknown): v is HomeDraftAttachment {
  if (!v || typeof v !== 'object') return false
  const a = v as Record<string, unknown>
  return typeof a.mimeType === 'string' && typeof a.data === 'string'
}

function normalizeAttachments(raw: unknown): HomeDraftAttachment[] {
  if (!Array.isArray(raw)) return []
  const out: HomeDraftAttachment[] = []
  for (const item of raw) {
    if (!isAttachment(item)) continue
    const next: HomeDraftAttachment = { mimeType: item.mimeType, data: item.data }
    if (typeof item.name === 'string' && item.name) next.name = item.name
    out.push(next)
  }
  return out
}

function parseDraft(raw: string): HomeComposerDraft | null {
  try {
    const parsed = JSON.parse(raw) as Partial<HomeComposerDraft>
    if (!parsed || typeof parsed !== 'object') return null
    if (typeof parsed.text !== 'string') return null
    if (typeof parsed.pipelineId !== 'string') return null
    if (typeof parsed.savedAt !== 'number' || !Number.isFinite(parsed.savedAt)) return null
    const attachments = normalizeAttachments(parsed.attachments)
    return {
      schemaVersion:
        typeof parsed.schemaVersion === 'string' && parsed.schemaVersion
          ? parsed.schemaVersion
          : HOME_COMPOSER_DRAFT_SCHEMA,
      savedAt: parsed.savedAt,
      pipelineId: parsed.pipelineId,
      text: parsed.text,
      attachments,
    }
  } catch {
    return null
  }
}

/** True when there is nothing worth persisting (empty text + no attachments). */
export function isHomeComposerDraftEmpty(text: string, attachments: ClarifyImage[]): boolean {
  return !text.trim() && attachments.length === 0
}

export function loadHomeComposerDraft(): HomeComposerDraft | null {
  try {
    const raw = localStorage.getItem(HOME_COMPOSER_DRAFT_KEY)
    if (!raw) return null
    return parseDraft(raw)
  } catch {
    return null
  }
}

export function clearHomeComposerDraft(): void {
  try {
    localStorage.removeItem(HOME_COMPOSER_DRAFT_KEY)
  } catch {
    /* ignore private mode */
  }
}

/**
 * Persist home composer draft. Empty content clears the key instead of writing a shell.
 * Returns ok | quota_exceeded | error (aligned with runDraft).
 */
function toPersistableAttachments(attachments: ClarifyImage[]): HomeDraftAttachment[] {
  const out: HomeDraftAttachment[] = []
  for (const im of attachments) {
    if (!im.data) continue
    const next: HomeDraftAttachment = { mimeType: im.mimeType, data: im.data }
    if (im.name) next.name = im.name
    out.push(next)
  }
  return out
}

export function saveHomeComposerDraft(
  text: string,
  attachments: ClarifyImage[],
  pipelineId: string,
): SaveHomeComposerDraftResult {
  const persistable = toPersistableAttachments(attachments)
  if (isHomeComposerDraftEmpty(text, persistable as ClarifyImage[])) {
    clearHomeComposerDraft()
    return 'ok'
  }
  const payload: HomeComposerDraft = {
    schemaVersion: HOME_COMPOSER_DRAFT_SCHEMA,
    savedAt: Date.now(),
    pipelineId: pipelineId || '',
    text,
    attachments: persistable,
  }
  try {
    localStorage.setItem(HOME_COMPOSER_DRAFT_KEY, JSON.stringify(payload))
    return 'ok'
  } catch (e: unknown) {
    const err = e as { name?: string; code?: number }
    if (err?.name === 'QuotaExceededError' || err?.code === 22) {
      // Prefer text + pipeline when attachments blow the quota.
      if (attachments.length > 0) {
        try {
          const slim: HomeComposerDraft = {
            ...payload,
            attachments: [],
          }
          localStorage.setItem(HOME_COMPOSER_DRAFT_KEY, JSON.stringify(slim))
          return 'quota_exceeded'
        } catch (e2: unknown) {
          const err2 = e2 as { name?: string; code?: number }
          if (err2?.name === 'QuotaExceededError' || err2?.code === 22) return 'quota_exceeded'
          return 'error'
        }
      }
      return 'quota_exceeded'
    }
    return 'error'
  }
}

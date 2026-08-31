import type { ClarifyImage } from '../shared/types'
import {
  blobToBase64,
  getDraftIdb,
  isQuotaError,
  tryBase64ToBlob,
  type DraftAttachmentRecord,
  type RunDraftRecord,
} from './draftIdb'

export interface RunDraftPayload {
  workflowId: string
  savedAt: number
  inputs: Record<string, string>
  images: Record<string, ClarifyImage[]>
}

/** ok = full save; partial = fields persisted without images; quota_exceeded / error. */
export type SaveRunDraftResult = 'ok' | 'partial' | 'quota_exceeded' | 'error'

const LEGACY_PREFIX = 'run-draft:'

let runMigrationDone = false

/** Test-only: re-run legacy localStorage migration. */
export function __resetRunDraftMigrationForTests(): void {
  runMigrationDone = false
}

function draftKey(workflowId: string): string {
  return `${LEGACY_PREFIX}${workflowId}`
}

function listLegacyKeys(): string[] {
  const keys: string[] = []
  try {
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i)
      if (k && k.startsWith(LEGACY_PREFIX)) keys.push(k)
    }
  } catch {
    /* ignore */
  }
  return keys
}

function readLegacy(workflowId: string): RunDraftPayload | null {
  try {
    const raw = localStorage.getItem(draftKey(workflowId))
    if (!raw) return null
    return JSON.parse(raw) as RunDraftPayload
  } catch {
    return null
  }
}

function clearLegacy(workflowId: string): void {
  try {
    localStorage.removeItem(draftKey(workflowId))
  } catch {
    /* ignore */
  }
}

function writeLegacyTextFallback(payload: RunDraftPayload): SaveRunDraftResult {
  const slim: RunDraftPayload = {
    ...payload,
    images: {},
  }
  try {
    localStorage.setItem(draftKey(payload.workflowId), JSON.stringify(slim))
    const hadImages = Object.values(payload.images).some((arr) => (arr || []).length > 0)
    return hadImages ? 'partial' : 'ok'
  } catch (e: unknown) {
    if (isQuotaError(e)) return 'quota_exceeded'
    return 'error'
  }
}

function flattenImages(
  workflowId: string,
  images: Record<string, ClarifyImage[]>,
): DraftAttachmentRecord[] {
  const out: DraftAttachmentRecord[] = []
  let sortIndex = 0
  for (const [fieldKey, list] of Object.entries(images)) {
    for (const im of list || []) {
      if (!im.data) continue
      const blob = tryBase64ToBlob(im.data, im.mimeType)
      if (!blob) continue
      const rec: DraftAttachmentRecord = {
        id: `run:${workflowId}:${fieldKey}:${sortIndex}`,
        ownerKind: 'run',
        ownerId: workflowId,
        mimeType: im.mimeType,
        data: blob,
        sizeBytes: blob.size,
        fieldKey,
        sortIndex,
      }
      if (im.name) rec.name = im.name
      out.push(rec)
      sortIndex++
    }
  }
  return out
}

async function inflateImages(rows: DraftAttachmentRecord[]): Promise<Record<string, ClarifyImage[]>> {
  const images: Record<string, ClarifyImage[]> = {}
  const sorted = rows.slice().sort((a, b) => (a.sortIndex ?? 0) - (b.sortIndex ?? 0))
  for (const row of sorted) {
    const key = row.fieldKey || '_'
    if (!images[key]) images[key] = []
    const data = await blobToBase64(row.data)
    const im: ClarifyImage = { mimeType: row.mimeType, data }
    if (row.name) im.name = row.name
    if (row.sizeBytes != null) im.sizeBytes = row.sizeBytes
    images[key].push(im)
  }
  return images
}

function isRunDraftEmpty(inputs: Record<string, string>, images: Record<string, ClarifyImage[]>): boolean {
  const hasText = Object.values(inputs).some((v) => String(v ?? '').trim())
  if (hasText) return false
  for (const list of Object.values(images)) {
    if ((list || []).some((im) => !!im.data)) return false
  }
  return true
}

async function migrateLegacyRunIfNeeded(): Promise<void> {
  if (runMigrationDone) return
  runMigrationDone = true
  const keys = listLegacyKeys()
  if (keys.length === 0) return
  const idb = getDraftIdb()
  for (const key of keys) {
    const workflowId = key.slice(LEGACY_PREFIX.length)
    if (!workflowId) continue
    const legacy = readLegacy(workflowId)
    if (!legacy) {
      clearLegacy(workflowId)
      continue
    }
    try {
      const existing = await idb.getRun(workflowId)
      if (existing) {
        // Keep newer LS text-fallback for load to prefer (review v2); only drop stale LS.
        if ((legacy.savedAt || 0) <= existing.record.savedAt) {
          clearLegacy(workflowId)
        }
        continue
      }
      const record: RunDraftRecord = {
        workflowId,
        savedAt: legacy.savedAt || Date.now(),
        inputsJson: JSON.stringify(legacy.inputs || {}),
      }
      await idb.putRun(record, flattenImages(workflowId, legacy.images || {}))
      clearLegacy(workflowId)
    } catch {
      /* leave legacy key on failure */
    }
  }
}

export async function loadRunDraft(workflowId: string): Promise<RunDraftPayload | null> {
  await migrateLegacyRunIfNeeded()
  const legacy = readLegacy(workflowId)
  try {
    const packed = await getDraftIdb().getRun(workflowId)
    if (packed && legacy) {
      // Prefer newer savedAt so quota-fallback LS fields win over stale IDB (review v2 / F4).
      if ((legacy.savedAt || 0) > packed.record.savedAt) {
        return legacy
      }
      clearLegacy(workflowId)
      return draftFromIdbRun(packed)
    }
    if (packed) {
      return draftFromIdbRun(packed)
    }
  } catch {
    /* fall through */
  }
  return legacy
}

async function draftFromIdbRun(packed: {
  record: RunDraftRecord
  attachments: DraftAttachmentRecord[]
}): Promise<RunDraftPayload> {
  let inputs: Record<string, string> = {}
  try {
    inputs = JSON.parse(packed.record.inputsJson || '{}') as Record<string, string>
  } catch {
    inputs = {}
  }
  return {
    workflowId: packed.record.workflowId,
    savedAt: packed.record.savedAt,
    inputs,
    images: await inflateImages(packed.attachments),
  }
}

export async function clearRunDraft(workflowId: string): Promise<void> {
  try {
    await getDraftIdb().deleteRun(workflowId)
  } catch {
    /* ignore */
  }
  clearLegacy(workflowId)
}

export async function saveRunDraft(
  workflowId: string,
  inputs: Record<string, string>,
  images: Record<string, ClarifyImage[]>,
): Promise<SaveRunDraftResult> {
  await migrateLegacyRunIfNeeded()
  if (isRunDraftEmpty(inputs, images)) {
    await clearRunDraft(workflowId)
    return 'ok'
  }
  const payload: RunDraftPayload = {
    workflowId,
    savedAt: Date.now(),
    inputs,
    images,
  }
  const record: RunDraftRecord = {
    workflowId,
    savedAt: payload.savedAt,
    inputsJson: JSON.stringify(inputs),
  }
  try {
    await getDraftIdb().putRun(record, flattenImages(workflowId, images))
    clearLegacy(workflowId)
    return 'ok'
  } catch (e: unknown) {
    const fb = writeLegacyTextFallback(payload)
    if (isQuotaError(e)) {
      return fb === 'ok' || fb === 'partial' ? 'quota_exceeded' : fb
    }
    if (fb === 'error' || fb === 'quota_exceeded') return fb
    const hadImages = Object.values(images).some((arr) => (arr || []).some((im) => !!im.data))
    return hadImages ? 'partial' : 'ok'
  }
}

export async function mergeRunDraft(
  workflowId: string,
  seedInputs: Record<string, string>,
  seedImages: Record<string, ClarifyImage[]>,
  fieldKeys: string[],
): Promise<{ inputs: Record<string, string>; images: Record<string, ClarifyImage[]>; restored: boolean }> {
  const draft = await loadRunDraft(workflowId)
  if (!draft) {
    return { inputs: { ...seedInputs }, images: { ...seedImages }, restored: false }
  }

  const inputs = { ...seedInputs }
  const images = { ...seedImages }

  for (const key of fieldKeys) {
    if (Object.prototype.hasOwnProperty.call(draft.inputs, key)) {
      inputs[key] = draft.inputs[key]
    }
    if (Object.prototype.hasOwnProperty.call(draft.images, key)) {
      images[key] = draft.images[key] ? [...draft.images[key]] : []
    }
  }

  return { inputs, images, restored: true }
}

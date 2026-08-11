import type { ClarifyImage } from '../shared/types'

export interface RunDraftPayload {
  workflowId: string
  savedAt: number
  inputs: Record<string, string>
  images: Record<string, ClarifyImage[]>
}

export type SaveRunDraftResult = 'ok' | 'quota_exceeded' | 'error'

function draftKey(workflowId: string): string {
  return `run-draft:${workflowId}`
}

export function loadRunDraft(workflowId: string): RunDraftPayload | null {
  try {
    const raw = localStorage.getItem(draftKey(workflowId))
    if (!raw) return null
    return JSON.parse(raw) as RunDraftPayload
  } catch {
    return null
  }
}

export function clearRunDraft(workflowId: string): void {
  localStorage.removeItem(draftKey(workflowId))
}

export function saveRunDraft(
  workflowId: string,
  inputs: Record<string, string>,
  images: Record<string, ClarifyImage[]>,
): SaveRunDraftResult {
  const payload: RunDraftPayload = {
    workflowId,
    savedAt: Date.now(),
    inputs,
    images,
  }
  try {
    localStorage.setItem(draftKey(workflowId), JSON.stringify(payload))
    return 'ok'
  } catch (e: unknown) {
    const err = e as { name?: string; code?: number }
    if (err?.name === 'QuotaExceededError' || err?.code === 22) {
      return 'quota_exceeded'
    }
    return 'error'
  }
}

export function mergeRunDraft(
  workflowId: string,
  seedInputs: Record<string, string>,
  seedImages: Record<string, ClarifyImage[]>,
  fieldKeys: string[],
): { inputs: Record<string, string>; images: Record<string, ClarifyImage[]>; restored: boolean } {
  const draft = loadRunDraft(workflowId)
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

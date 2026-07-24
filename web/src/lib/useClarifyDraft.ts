import { reactive, computed } from 'vue'
import type { ClarifyImage, ReactAnnotation } from '@/lib/types'
import { pushAnnotationUnique, type AddAnnotationResult } from '@/lib/reviewQuote'

interface ClarifyDraftEntry {
  draft: string
  attachments: ClarifyImage[]
  // Pending field/element annotations staged for the next review reply. Shared
  // between the product view's ⤴ pick affordance and the review chat composer.
  annotations: ReactAnnotation[]
}

const store = reactive<Record<string, ClarifyDraftEntry>>({})

function draftKey(runId: string, nodeId: string) {
  return `${runId}:${nodeId}`
}

function ensureEntry(runId: string, nodeId: string): ClarifyDraftEntry {
  const k = draftKey(runId, nodeId)
  if (!store[k]) {
    store[k] = { draft: '', attachments: [], annotations: [] }
  }
  return store[k]
}

function resolveRunId(runId: string | (() => string | null)): string | null {
  return typeof runId === 'function' ? runId() : runId
}

/** Parent-scoped draft + attachments keyed by runId:nodeId (survives Tab remount). */
export function useClarifyDraft(runId: string | (() => string | null), nodeId: () => string | null) {
  const draft = computed({
    get() {
      const rid = resolveRunId(runId)
      const id = nodeId()
      if (!rid || !id) return ''
      return ensureEntry(rid, id).draft
    },
    set(v: string) {
      const rid = resolveRunId(runId)
      const id = nodeId()
      if (rid && id) ensureEntry(rid, id).draft = v
    },
  })

  const attachments = computed({
    get() {
      const rid = resolveRunId(runId)
      const id = nodeId()
      if (!rid || !id) return [] as ClarifyImage[]
      return ensureEntry(rid, id).attachments
    },
    set(v: ClarifyImage[]) {
      const rid = resolveRunId(runId)
      const id = nodeId()
      if (rid && id) ensureEntry(rid, id).attachments = v
    },
  })

  const annotations = computed({
    get() {
      const rid = resolveRunId(runId)
      const id = nodeId()
      if (!rid || !id) return [] as ReactAnnotation[]
      return ensureEntry(rid, id).annotations
    },
    set(v: ReactAnnotation[]) {
      const rid = resolveRunId(runId)
      const id = nodeId()
      if (rid && id) ensureEntry(rid, id).annotations = v
    },
  })

  return { draft, attachments, annotations }
}

/**
 * Append one annotation chip to a node's staged review annotations.
 * Quote excerpts dedupe by quote+path; whole-field / selector still by path.
 */
export function addClarifyAnnotation(
  runId: string,
  nodeId: string,
  ann: ReactAnnotation,
): AddAnnotationResult {
  if (!runId || !nodeId) return 'ignored'
  const entry = ensureEntry(runId, nodeId)
  return pushAnnotationUnique(entry.annotations, ann)
}

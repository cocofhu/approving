import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { InputField } from '@/components/workflow/RunLaunchModal.vue'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import { useImageAttachments } from '@/lib/composables/useImageAttachments'
import { readStoredProjectId } from '@/lib/composables/useProjectContext'
import { approveFirstNodeId, isPublishedApproveFirst } from '@/lib/run/approveFirstPipeline'
import {
  clearHomeComposerDraft,
  loadHomeComposerDraft,
  saveHomeComposerDraft,
} from '@/lib/run/homeComposerDraft'
import { setHomeApproveHandoff } from '@/lib/run/homeApproveHandoff'
import { clipRunTitle } from '@/lib/run/runTitle'
import { missingRequiredAskField, seedAskLaunchFields } from '@/lib/run/useWorkflowAskInputs'
import { attachmentDisplayName } from '@/lib/shared/attachments'
import type { ClarifyImage, Workflow } from '@/lib/shared/types'

/** Remember last selected home pipeline across visits (plan g2.4). */
export const HOME_PIPELINE_MEMORY_KEY = 'approving.home.lastPipelineId'

/** Debounce for auto-save (plan g2.2; NFR ~300–800ms). */
export const HOME_COMPOSER_DRAFT_DEBOUNCE_MS = 400

function titleFromDraft(text: string, images: ClarifyImage[]): string {
  const clipped = clipRunTitle(text)
  if (clipped) return clipped
  const name = images[0] ? attachmentDisplayName(images[0], 0) : ''
  return clipRunTitle(name)
}

function readLastPipelineId(): string {
  try {
    return localStorage.getItem(HOME_PIPELINE_MEMORY_KEY)?.trim() || ''
  } catch {
    return ''
  }
}

function writeLastPipelineId(id: string) {
  try {
    if (id) localStorage.setItem(HOME_PIPELINE_MEMORY_KEY, id)
    else localStorage.removeItem(HOME_PIPELINE_MEMORY_KEY)
  } catch {
    /* ignore quota / private mode */
  }
}

function pickDefaultPipelineId(list: Workflow[], preferred: string): string {
  if (preferred && list.some((w) => w.id === preferred)) return preferred
  return list[0]?.id || ''
}

export function useHomeApproveChat() {
  const router = useRouter()
  const toast = useToast()
  const { t } = useI18n()
  const attach = useImageAttachments()

  const workflows = ref<Workflow[]>([])
  const loading = ref(false)
  const loadError = ref<string | null>(null)
  const selectedId = ref('')
  const draft = ref('')
  const sending = ref(false)
  const pendingText = ref('')
  const pendingImages = ref<ClarifyImage[]>([])

  const launchOpen = ref(false)
  const launchTarget = ref<Workflow | null>(null)
  const runFields = ref<InputField[]>([])
  const runInputs = ref<Record<string, string>>({})
  const runImages = ref<Record<string, ClarifyImage[]>>({})
  const draftRestored = ref(false)

  let loadAbort: AbortController | null = null
  let saveTimer: ReturnType<typeof setTimeout> | null = null
  /** Skip auto-save while applying a restored draft. */
  let suppressSave = false
  /** Draft pipeline id preferred over HOME_PIPELINE_MEMORY_KEY (plan g2.4). */
  let preferredDraftPipelineId = ''
  /** Toast once after a non-empty restore. */
  let restoreToastShown = false

  const pipelines = computed(() => workflows.value.filter(isPublishedApproveFirst))
  const selected = computed(
    () => pipelines.value.find((w) => w.id === selectedId.value) || pipelines.value[0] || null,
  )
  /** Project context from the selected pipeline (not a home project gate). */
  const projectId = computed(
    () => selected.value?.projectId || launchTarget.value?.projectId || readStoredProjectId() || '',
  )
  const launchTitle = computed(() => titleFromDraft(pendingText.value, pendingImages.value))
  /** Opening message carried through the launch modal's own startRun call. */
  const launchFirstMessage = computed(() => ({
    text: pendingText.value,
    images: pendingImages.value,
  }))
  const canSend = computed(() => !!draft.value.trim() || attach.attachments.value.length > 0)

  function flushComposerDraftSave() {
    if (suppressSave) return
    const result = saveHomeComposerDraft(
      draft.value,
      attach.attachments.value,
      selectedId.value || preferredDraftPipelineId || '',
    )
    if (result === 'quota_exceeded') toast.error(t('common.toast.draftTooLarge'))
    else if (result === 'error') toast.error(t('common.toast.draftSaveFailed'))
  }

  function scheduleComposerDraftSave() {
    if (suppressSave) return
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(() => {
      saveTimer = null
      flushComposerDraftSave()
    }, HOME_COMPOSER_DRAFT_DEBOUNCE_MS)
  }

  function clearComposerDraftNow() {
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    clearHomeComposerDraft()
    preferredDraftPipelineId = ''
  }

  /**
   * Silent restore from localStorage (plan g2.1).
   * Pipeline selection: draft pipeline > lastPipeline memory > list default (g2.4).
   */
  function hydrateComposerDraft() {
    const stored = loadHomeComposerDraft()
    if (!stored) return
    const hasContent = !!stored.text.trim() || stored.attachments.length > 0
    if (!hasContent) {
      clearHomeComposerDraft()
      return
    }

    suppressSave = true
    try {
      draft.value = stored.text
      attach.attachments.value = stored.attachments.map((im) => ({ ...im }))
      if (stored.pipelineId) {
        preferredDraftPipelineId = stored.pipelineId
        selectedId.value = stored.pipelineId
      }
      if (!restoreToastShown) {
        restoreToastShown = true
        toast.success(t('pages.dashboard.draftRestored'))
      }
    } finally {
      suppressSave = false
    }
  }

  watch(
    pipelines,
    (list) => {
      const draftPreferred =
        preferredDraftPipelineId && list.some((w) => w.id === preferredDraftPipelineId)
          ? preferredDraftPipelineId
          : ''
      // Drop stale draft preference so memory/default can apply (plan g2.4).
      if (preferredDraftPipelineId && !draftPreferred) {
        preferredDraftPipelineId = ''
      }
      const selectedPreferred =
        selectedId.value && list.some((w) => w.id === selectedId.value) ? selectedId.value : ''
      const preferred = draftPreferred || selectedPreferred || readLastPipelineId()
      const next = pickDefaultPipelineId(list, preferred)
      if (next !== selectedId.value) selectedId.value = next
      if (next) writeLastPipelineId(next)
    },
    { immediate: true },
  )

  watch([draft, () => attach.attachments.value, selectedId], () => {
    scheduleComposerDraftSave()
  }, { deep: true })

  async function load() {
    loadAbort?.abort()
    const ac = new AbortController()
    loadAbort = ac
    loading.value = true
    loadError.value = null
    try {
      // Cross-project: omit projectId so the API returns all visible workflows.
      const list = await api.listWorkflows({ signal: ac.signal })
      if (ac.signal.aborted) return
      workflows.value = Array.isArray(list) ? list : []
    } catch (e: any) {
      if (ac.signal.aborted || e?.name === 'AbortError') return
      loadError.value = String(e?.message || e)
      workflows.value = []
    } finally {
      if (!ac.signal.aborted) loading.value = false
    }
  }

  function selectPipeline(id: string) {
    if (!id) return
    selectedId.value = id
    preferredDraftPipelineId = id
    writeLastPipelineId(id)
  }

  function seedLaunch(wf: Workflow) {
    const seeded = seedAskLaunchFields(wf)
    launchTarget.value = wf
    runFields.value = seeded.fields
    runInputs.value = seeded.inputs
    runImages.value = seeded.images
    draftRestored.value = seeded.restored
    launchOpen.value = true
  }

  function closeLaunch() {
    launchOpen.value = false
  }

  function goGates(runId: string, nodeId?: string) {
    return router.push({
      path: '/gates',
      query: nodeId ? { run: runId, node: nodeId } : { run: runId },
    })
  }

  /**
   * The message travels with startRun and is delivered into the sandbox by the
   * engine once the approve node parks, so all we do here is hand the optimistic
   * bubble to the inbox and navigate.
   */
  async function afterStart(runId: string, text: string, images: ClarifyImage[]) {
    const wf = selected.value || launchTarget.value
    const knownNodeId = wf ? approveFirstNodeId(wf) || '' : ''
    setHomeApproveHandoff({ runId, nodeId: knownNodeId, text, images })
    await goGates(runId, knownNodeId || undefined)
  }

  async function send() {
    const wf = selected.value
    const text = draft.value.trim()
    const images = attach.attachments.value.map((im) => ({ ...im }))
    if (!wf) {
      toast.warn(t('pages.dashboard.pickPipeline'))
      return
    }
    if (!text && images.length === 0) {
      toast.warn(t('pages.dashboard.needText'))
      return
    }
    if (attach.blockSendIfOversized(images)) return
    if (sending.value) return
    sending.value = true
    pendingText.value = text
    pendingImages.value = images
    writeLastPipelineId(wf.id)
    try {
      const missing = missingRequiredAskField(wf)
      if (missing) {
        toast.warn(t('pages.dashboard.missingRequired'))
        seedLaunch(wf)
        return
      }
      const res = await api.startRun(wf.id, {}, 'manual', 'normal', [], {
        title: titleFromDraft(text, images),
        firstMessage: { text, images },
      })
      // Success path: clear composer + local draft (plan g2.3).
      suppressSave = true
      draft.value = ''
      attach.clearAttachments()
      clearComposerDraftNow()
      suppressSave = false
      await afterStart(res.id, text, images)
    } catch (e: any) {
      const msg = String(e?.message || e)
      if (msg.includes('缺少必填项')) {
        toast.warn(t('pages.dashboard.missingRequired'))
        seedLaunch(wf)
        return
      }
      toast.error(msg)
    } finally {
      sending.value = false
    }
  }

  async function onLaunchStarted(runId: string) {
    launchOpen.value = false
    const text = pendingText.value
    const images = pendingImages.value.map((im) => ({ ...im }))
    sending.value = true
    try {
      // RunLaunch completed startRun — clear composer + draft (plan g2.3).
      suppressSave = true
      draft.value = ''
      attach.clearAttachments()
      clearComposerDraftNow()
      suppressSave = false
      await afterStart(runId, text, images)
    } finally {
      sending.value = false
    }
  }

  onMounted(() => {
    hydrateComposerDraft()
    void load()
  })
  onUnmounted(() => {
    loadAbort?.abort()
    if (saveTimer) clearTimeout(saveTimer)
  })

  return {
    projectId,
    pipelines,
    selected,
    selectedId,
    draft,
    sending,
    canSend,
    loading,
    loadError,
    launchOpen,
    launchTarget,
    launchTitle,
    launchFirstMessage,
    runFields,
    runInputs,
    runImages,
    draftRestored,
    attachments: attach.attachments,
    fileInput: attach.fileInput,
    attachNotice: attach.notice,
    onPickFiles: attach.onPickFiles,
    onPaste: attach.onPaste,
    removeAttachment: attach.removeAttachment,
    load,
    selectPipeline,
    send,
    closeLaunch,
    onLaunchStarted,
  }
}

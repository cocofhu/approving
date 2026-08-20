import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { InputField } from '@/components/workflow/RunLaunchModal.vue'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import { useImageAttachments } from '@/lib/composables/useImageAttachments'
import { readStoredProjectId } from '@/lib/composables/useProjectContext'
import { approveFirstNodeId, isPublishedApproveFirst } from '@/lib/run/approveFirstPipeline'
import { setHomeApproveHandoff } from '@/lib/run/homeApproveHandoff'
import { clipRunTitle } from '@/lib/run/runTitle'
import { missingRequiredAskField, seedAskLaunchFields } from '@/lib/run/useWorkflowAskInputs'
import { attachmentDisplayName } from '@/lib/shared/attachments'
import type { ClarifyImage, Workflow } from '@/lib/shared/types'

function titleFromDraft(text: string, images: ClarifyImage[]): string {
  const clipped = clipRunTitle(text)
  if (clipped) return clipped
  const name = images[0] ? attachmentDisplayName(images[0], 0) : ''
  return clipRunTitle(name)
}

export function useHomeApproveChat() {
  const router = useRouter()
  const toast = useToast()
  const { t } = useI18n()
  const attach = useImageAttachments()

  const projectId = ref(readStoredProjectId())
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

  const hasProject = computed(() => !!projectId.value)
  const pipelines = computed(() => workflows.value.filter(isPublishedApproveFirst))
  const selected = computed(
    () => pipelines.value.find((w) => w.id === selectedId.value) || pipelines.value[0] || null,
  )
  const launchTitle = computed(() => titleFromDraft(pendingText.value, pendingImages.value))
  /** Opening message carried through the launch modal's own startRun call. */
  const launchFirstMessage = computed(() => ({
    text: pendingText.value,
    images: pendingImages.value,
  }))
  const canSend = computed(() => !!draft.value.trim() || attach.attachments.value.length > 0)

  watch(
    pipelines,
    (list) => {
      if (!list.some((w) => w.id === selectedId.value)) {
        selectedId.value = list[0]?.id || ''
      }
    },
    { immediate: true },
  )

  async function load() {
    loadAbort?.abort()
    projectId.value = readStoredProjectId()
    if (!projectId.value) {
      workflows.value = []
      loadError.value = null
      loading.value = false
      return
    }
    const ac = new AbortController()
    loadAbort = ac
    loading.value = true
    loadError.value = null
    try {
      const list = await api.listWorkflows({ projectId: projectId.value, signal: ac.signal })
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
    selectedId.value = id
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
      draft.value = ''
      attach.clearAttachments()
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
      draft.value = ''
      attach.clearAttachments()
      await afterStart(runId, text, images)
    } finally {
      sending.value = false
    }
  }

  onMounted(() => {
    void load()
  })
  onUnmounted(() => {
    loadAbort?.abort()
  })

  return {
    projectId,
    hasProject,
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

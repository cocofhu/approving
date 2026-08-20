import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { InputField } from '@/components/workflow/RunLaunchModal.vue'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import { readStoredProjectId } from '@/lib/composables/useProjectContext'
import { isPublishedApproveFirst } from '@/lib/run/approveFirstPipeline'
import { ApproveParkTimeout, waitForApprovePark } from '@/lib/run/homeApproveChat'
import { clipRunTitle } from '@/lib/run/runTitle'
import { missingRequiredAskField, seedAskLaunchFields } from '@/lib/run/useWorkflowAskInputs'
import type { ClarifyImage, Workflow } from '@/lib/shared/types'

export function useHomeApproveChat() {
  const router = useRouter()
  const toast = useToast()
  const { t } = useI18n()

  const projectId = ref(readStoredProjectId())
  const workflows = ref<Workflow[]>([])
  const loading = ref(false)
  const loadError = ref<string | null>(null)
  const selectedId = ref('')
  const draft = ref('')
  const sending = ref(false)
  const pendingText = ref('')

  const launchOpen = ref(false)
  const launchTarget = ref<Workflow | null>(null)
  const runFields = ref<InputField[]>([])
  const runInputs = ref<Record<string, string>>({})
  const runImages = ref<Record<string, ClarifyImage[]>>({})
  const draftRestored = ref(false)

  let loadAbort: AbortController | null = null
  let parkAbort: AbortController | null = null

  const hasProject = computed(() => !!projectId.value)
  const pipelines = computed(() => workflows.value.filter(isPublishedApproveFirst))
  const selected = computed(
    () => pipelines.value.find((w) => w.id === selectedId.value) || pipelines.value[0] || null,
  )
  const launchTitle = computed(() => clipRunTitle(pendingText.value))

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

  async function afterStart(runId: string, text: string) {
    parkAbort?.abort()
    const ac = new AbortController()
    parkAbort = ac
    try {
      const { nodeId } = await waitForApprovePark(runId, { signal: ac.signal })
      await api.reactReply(runId, nodeId, text)
      await router.push({ path: `/runs/${runId}`, query: { node: nodeId } })
    } catch (e: any) {
      if (ac.signal.aborted || e?.name === 'AbortError') return
      if (e instanceof ApproveParkTimeout) {
        toast.warn(t('pages.dashboard.parkTimeout'))
        await router.push({ path: `/runs/${runId}` })
        return
      }
      toast.error(String(e?.message || e))
      await router.push({ path: `/runs/${runId}` })
    }
  }

  async function send() {
    const wf = selected.value
    const text = draft.value.trim()
    if (!wf) {
      toast.warn(t('pages.dashboard.pickPipeline'))
      return
    }
    if (!text) {
      toast.warn(t('pages.dashboard.needText'))
      return
    }
    if (sending.value) return
    sending.value = true
    pendingText.value = text
    try {
      const missing = missingRequiredAskField(wf)
      if (missing) {
        toast.warn(t('pages.dashboard.missingRequired'))
        seedLaunch(wf)
        return
      }
      const res = await api.startRun(wf.id, {}, 'manual', 'normal', [], { title: clipRunTitle(text) })
      draft.value = ''
      await afterStart(res.id, text)
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
    sending.value = true
    try {
      draft.value = ''
      await afterStart(runId, text)
    } finally {
      sending.value = false
    }
  }

  onMounted(() => {
    void load()
  })
  onUnmounted(() => {
    loadAbort?.abort()
    parkAbort?.abort()
  })

  return {
    projectId,
    hasProject,
    pipelines,
    selected,
    selectedId,
    draft,
    sending,
    loading,
    loadError,
    launchOpen,
    launchTarget,
    launchTitle,
    runFields,
    runInputs,
    runImages,
    draftRestored,
    load,
    selectPipeline,
    send,
    closeLaunch,
    onLaunchStarted,
  }
}

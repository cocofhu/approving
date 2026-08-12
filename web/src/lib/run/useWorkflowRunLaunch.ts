import { ref } from 'vue'
import type { InputField } from '@/components/workflow/RunLaunchModal.vue'
import { clearRunDraft, mergeRunDraft, saveRunDraft } from '@/lib/run/runDraft'
import { useToast } from '@/lib/composables/useToast'
import type { ClarifyImage, Workflow } from '@/lib/shared/types'
import { useI18n } from 'vue-i18n'

function askFieldsFromWorkflow(w: Workflow): InputField[] {
  const input = (w.nodes || []).find((n) => n.type === 'input')
  const vars = ((input?.config?.variables as any[]) || []).filter((v) => v && v.name && v.ask)
  return vars.map((v) => ({
    key: v.name,
    desc: v.desc,
    type: v.type === 'string' ? 'text' : v.type,
    required: v.required,
    default:
      v.type === 'repos'
        ? JSON.stringify(Array.isArray(v.value) ? v.value : [])
        : v.value == null
          ? ''
          : String(v.value),
    editable: v.editable,
    options: v.options,
  }))
}

function fieldOptions(f: InputField): string[] {
  return String(f.options || '')
    .split(/[,，]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

const open = ref(false)
const target = ref<Workflow | null>(null)
const runFields = ref<InputField[]>([])
const runInputs = ref<Record<string, string>>({})
const runImages = ref<Record<string, ClarifyImage[]>>({})
const draftRestored = ref(false)

/**
 * Shell-level RunLaunchModal orchestrator so sidebar quick-launch works on any route.
 */
export function useWorkflowRunLaunch() {
  const toast = useToast()
  const { t } = useI18n()

  function openLaunch(workflow: Workflow) {
    target.value = workflow
    draftRestored.value = false
    runFields.value = askFieldsFromWorkflow(workflow)
    const seed: Record<string, string> = {}
    const imgSeed: Record<string, ClarifyImage[]> = {}
    for (const f of runFields.value) {
      seed[f.key] = f.default || (f.type === 'select' ? fieldOptions(f)[0] || '' : '')
      imgSeed[f.key] = []
    }
    const keys = runFields.value.map((f) => f.key)
    const merged = mergeRunDraft(workflow.id, seed, imgSeed, keys)
    runInputs.value = merged.inputs
    runImages.value = merged.images
    draftRestored.value = merged.restored
    open.value = true
  }

  function closeLaunch() {
    open.value = false
    target.value = null
  }

  function saveRunDraftClick() {
    const wf = target.value
    if (!wf) return
    const images: Record<string, ClarifyImage[]> = {}
    for (const [k, v] of Object.entries(runImages.value)) {
      images[k] = v ? [...v] : []
    }
    const result = saveRunDraft(wf.id, { ...runInputs.value }, images)
    if (result === 'ok') toast.success(t('common.toast.draftSaved'))
    else if (result === 'quota_exceeded') toast.error(t('common.toast.draftTooLarge'))
    else toast.error(t('common.toast.draftSaveFailed'))
  }

  function onStarted() {
    if (target.value) clearRunDraft(target.value.id)
  }

  return {
    open,
    target,
    runFields,
    runInputs,
    runImages,
    draftRestored,
    openLaunch,
    closeLaunch,
    saveRunDraftClick,
    onStarted,
  }
}

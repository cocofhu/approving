import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import { missingSkillProfiles } from '@/lib/workflowIO'
import { readStoredProjectId } from '@/lib/useProjectContext'
import { useToast } from '@/lib/useToast'
import type { Workflow } from '@/lib/types'

export function useWorkflowImport(opts?: {
  dirty?: () => boolean
  onConfirmDiscard?: () => Promise<boolean>
  projectId?: () => string | undefined
  /** When set, called after a successful import instead of navigating to the editor. */
  onImported?: (wf: Workflow) => void | Promise<void>
}) {
  const router = useRouter()
  const { t } = useI18n()
  const toast = useToast()
  const fileInput = ref<HTMLInputElement | null>(null)
  const showDiscardConfirm = ref(false)
  let pendingFile: File | null = null

  function triggerImport() {
    if (opts?.dirty?.()) {
      showDiscardConfirm.value = true
      return
    }
    fileInput.value?.click()
  }

  function onDiscardCancel() {
    showDiscardConfirm.value = false
    pendingFile = null
  }

  function onDiscardConfirm() {
    showDiscardConfirm.value = false
    fileInput.value?.click()
  }

  async function handleFileChange(e: Event) {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file) return

    try {
      const text = await file.text()
      const projectId = opts?.projectId?.() || readStoredProjectId() || undefined
      const wf = await api.importWorkflow(text, projectId)
      await warnMissingAgents(wf)
      toast.success(t('pages.workflowIO.import.success', { name: wf.name }))
      if (opts?.onImported) {
        await opts.onImported(wf)
      } else {
        router.push('/workflows/' + wf.id + '/edit')
      }
    } catch (err: any) {
      toast.error(String(err?.message || err))
    }
  }

  async function warnMissingAgents(wf: Workflow) {
    try {
      const agents = await api.listAgents()
      const missing = missingSkillProfiles(wf.nodes, agents.map((a) => a.name))
      if (missing.length) {
        toast.warn(t('pages.workflowIO.import.missingSkillProfiles', { list: missing.join('、') }))
      }
    } catch {
      // non-blocking
    }
  }

  return {
    fileInput,
    showDiscardConfirm,
    triggerImport,
    onDiscardCancel,
    onDiscardConfirm,
    handleFileChange,
  }
}

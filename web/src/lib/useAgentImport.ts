import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import {
  peekAgentZipName,
  resolveImportName,
  suggestRename,
  validateAgentName,
  normalizeAgentName,
} from '@/lib/agentIO'
import { useToast } from '@/lib/useToast'
import type { Agent } from '@/lib/api'

export type ConflictAction = 'overwrite' | 'rename' | 'cancel'

export function useAgentImport(opts: {
  dirty: () => boolean
  agentNames: () => string[]
  onImported: (agent: Agent) => void | Promise<void>
}) {
  const { t } = useI18n()
  const toast = useToast()

  const fileInput = ref<HTMLInputElement | null>(null)
  const showDiscardConfirm = ref(false)
  const showConflict = ref(false)
  const showImportError = ref(false)
  const importError = ref('')

  const conflictName = ref('')
  const conflictAction = ref<ConflictAction>('rename')
  const renameValue = ref('')
  const renameError = ref('')

  let pendingFile: File | null = null

  function triggerImport() {
    if (opts.dirty()) {
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

    if (!file.name.toLowerCase().endsWith('.zip')) {
      importError.value = t('pages.agentStudio.exportImport.importError.invalidZip')
      showImportError.value = true
      return
    }

    pendingFile = file
    const peek = await peekAgentZipName(file)
    if (peek.error) {
      importError.value =
        peek.error === 'missing agent.json'
          ? t('pages.agentStudio.exportImport.importError.missingAgentJson')
          : t('pages.agentStudio.exportImport.importError.invalidZip')
      showImportError.value = true
      pendingFile = null
      return
    }

    const targetName = normalizeAgentName(resolveImportName(peek.name, file.name))
    const nameErr = validateAgentName(targetName)
    if (nameErr === 'required' || nameErr === 'invalid') {
      importError.value = t('pages.agentStudio.exportImport.importError.invalidName')
      showImportError.value = true
      pendingFile = null
      return
    }

    const existing = opts.agentNames()
    if (existing.includes(targetName)) {
      conflictName.value = targetName
      conflictAction.value = 'rename'
      renameValue.value = suggestRename(targetName, existing)
      renameError.value = ''
      showConflict.value = true
      return
    }

    await runImport(targetName, 'create')
  }

  function selectConflict(action: ConflictAction) {
    conflictAction.value = action
    renameError.value = ''
  }

  function closeConflict() {
    showConflict.value = false
    pendingFile = null
  }

  async function confirmConflict() {
    if (conflictAction.value === 'cancel') {
      closeConflict()
      return
    }

    const file = pendingFile
    if (!file) {
      closeConflict()
      return
    }

    if (conflictAction.value === 'overwrite') {
      showConflict.value = false
      await runImport(conflictName.value, 'overwrite')
      return
    }

    const newName = normalizeAgentName(renameValue.value)
    const err = validateAgentName(newName)
    if (err === 'required' || err === 'invalid') {
      renameError.value = t('pages.agentStudio.exportImport.conflict.nameInvalid')
      return
    }
    if (opts.agentNames().includes(newName)) {
      renameError.value = t('pages.agentStudio.exportImport.conflict.nameExists')
      return
    }

    showConflict.value = false
    await runImport(newName, 'create')
  }

  async function runImport(targetName: string, mode: 'create' | 'overwrite') {
    const file = pendingFile
    pendingFile = null
    if (!file) return

    try {
      const agent = await api.importAgent(file, { targetName, mode })
      await opts.onImported(agent)
      toast.success(t('pages.agentStudio.exportImport.importSuccess', { name: targetName }))
    } catch (err: any) {
      importError.value = String(err?.message || err)
      showImportError.value = true
    }
  }

  return {
    fileInput,
    showDiscardConfirm,
    showConflict,
    showImportError,
    importError,
    conflictName,
    conflictAction,
    renameValue,
    renameError,
    triggerImport,
    onDiscardCancel,
    onDiscardConfirm,
    handleFileChange,
    selectConflict,
    closeConflict,
    confirmConflict,
  }
}

import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, type Agent, type AgentOrg } from '@/lib/api'
import {
  peekAgentZipName,
  peekZipPackage,
  resolveImportName,
  suggestRename,
  validateAgentName,
  normalizeAgentName,
} from '@/lib/agentIO'
import { useToast } from '@/lib/useToast'

export type ConflictAction = 'overwrite' | 'rename' | 'cancel'
export type FolderConflictAction = 'rename' | 'overwrite' | 'cancel'

export function useAgentImport(opts: {
  dirty: () => boolean
  agentDirty?: () => boolean
  orgDirty?: () => boolean
  persistOrg?: () => Promise<boolean>
  agentNames: () => string[]
  onImported: (agent: Agent) => void | Promise<void>
  onFolderImported?: (org: AgentOrg) => void | Promise<void>
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

  const showBatchConflict = ref(false)
  const batchConflictNames = ref<string[]>([])

  let pendingFile: File | null = null
  let pendingTargetGroupId: string | null = null
  let requireFolder = false

  function agentIsDirty() {
    return opts.agentDirty ? opts.agentDirty() : opts.dirty()
  }

  async function persistOrgIfNeeded(): Promise<boolean> {
    if (!opts.orgDirty?.()) return true
    if (!opts.persistOrg) return true
    return opts.persistOrg()
  }

  function triggerImport() {
    pendingTargetGroupId = null
    requireFolder = false
    void (async () => {
      if (!(await persistOrgIfNeeded())) return
      if (agentIsDirty()) {
        showDiscardConfirm.value = true
        return
      }
      fileInput.value?.click()
    })()
  }

  function triggerGroupImport(groupId: string) {
    pendingTargetGroupId = groupId
    requireFolder = true
    void (async () => {
      if (!(await persistOrgIfNeeded())) return
      if (agentIsDirty()) {
        showDiscardConfirm.value = true
        return
      }
      fileInput.value?.click()
    })()
  }

  function onDiscardCancel() {
    showDiscardConfirm.value = false
    pendingFile = null
    pendingTargetGroupId = null
    requireFolder = false
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
    const peek = await peekZipPackage(file)

    if (peek.kind === 'unknown') {
      importError.value =
        peek.error === 'invalid zip'
          ? t('pages.agentStudio.exportImport.importError.invalidZip')
          : t('pages.agentStudio.exportImport.importError.unrecognized')
      showImportError.value = true
      pendingFile = null
      return
    }

    if (requireFolder) {
      if (peek.kind === 'agent') {
        importError.value = t('pages.agentStudio.exportImport.importError.singleAgentOnGroup')
        showImportError.value = true
        pendingFile = null
        return
      }
      await beginFolderImport(peek.agentNames)
      return
    }

    if (peek.kind === 'org-folder') {
      await beginFolderImport(peek.agentNames)
      return
    }

    await beginSingleAgentImport(file, peek.name)
  }

  async function beginSingleAgentImport(file: File, peekedName?: string) {
    const peek = peekedName != null ? { name: peekedName } : await peekAgentZipName(file)
    if ('error' in peek && peek.error && peekedName == null) {
      importError.value =
        peek.error === 'missing agent.json'
          ? t('pages.agentStudio.exportImport.importError.missingAgentJson')
          : t('pages.agentStudio.exportImport.importError.invalidZip')
      showImportError.value = true
      pendingFile = null
      return
    }

    const targetName = normalizeAgentName(resolveImportName(peekedName ?? peek.name, file.name))
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

  async function beginFolderImport(agentNames: string[]) {
    const existing = new Set(opts.agentNames())
    const conflicts = agentNames.filter((n) => existing.has(n))
    if (conflicts.length > 0) {
      batchConflictNames.value = conflicts
      showBatchConflict.value = true
      return
    }
    await runFolderImport('rename')
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

  function closeBatchConflict() {
    showBatchConflict.value = false
    pendingFile = null
    pendingTargetGroupId = null
    requireFolder = false
    batchConflictNames.value = []
  }

  async function confirmBatchRename() {
    showBatchConflict.value = false
    await runFolderImport('rename')
  }

  async function confirmBatchOverwrite() {
    showBatchConflict.value = false
    await runFolderImport('overwrite')
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

  async function runFolderImport(mode: 'rename' | 'overwrite') {
    const file = pendingFile
    const targetGroupId = pendingTargetGroupId
    pendingFile = null
    pendingTargetGroupId = null
    requireFolder = false
    if (!file) return

    try {
      const result = await api.importOrgFolder(file, {
        targetGroupId: targetGroupId || undefined,
        mode,
      })
      if (opts.onFolderImported) {
        await opts.onFolderImported(result.org)
      }
      toast.success(t('pages.agentStudio.exportImport.folderImportSuccess'))
    } catch (err: any) {
      const msg = String(err?.message || err)
      importError.value = msg.includes('整次回滚')
        ? msg
        : `${t('pages.agentStudio.exportImport.importError.rolledBack')} ${msg}`
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
    showBatchConflict,
    batchConflictNames,
    triggerImport,
    triggerGroupImport,
    onDiscardCancel,
    onDiscardConfirm,
    handleFileChange,
    selectConflict,
    closeConflict,
    confirmConflict,
    closeBatchConflict,
    confirmBatchRename,
    confirmBatchOverwrite,
  }
}

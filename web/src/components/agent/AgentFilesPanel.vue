<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AppButton from '@/components/ui/AppButton.vue'
import CodeEditor from '@/components/ui/CodeEditor.vue'
import MarkdownSplitEditor from '@/components/agent/MarkdownSplitEditor.vue'
import ExplorerContextMenu, { type CtxTarget } from '@/components/agent/ExplorerContextMenu.vue'
import type { AgentStudioDraft, DraftFile } from '@/lib/agent/agentStudioDraft'

export type FilesStep = 'list' | 'edit'
export type FilesOpenSnapshot = { path: string; openPaths: string[] }

const props = defineProps<{
  draft: AgentStudioDraft
  dirty: boolean
  isMobile: boolean
  save: () => Promise<boolean>
}>()

const emit = defineEmits<{
  error: [message: string]
  toast: [message: string]
  'update:just-saved': [value: boolean]
  discard: []
}>()

const { t } = useI18n()

const EXPLORER_COLLAPSED_KEY = 'agent-studio-explorer-collapsed'
const SIDEBAR_EXPANDED_W = '240px'
const SIDEBAR_COLLAPSED_W = '28px'

function readCollapsedState(key: string): boolean {
  try {
    const v = localStorage.getItem(key)
    if (v === null) return false
    return v === 'true'
  } catch {
    return false
  }
}

function writeCollapsedState(key: string, collapsed: boolean) {
  try {
    localStorage.setItem(key, String(collapsed))
  } catch {
    /* ignore */
  }
}

const explorerCollapsed = ref(false)
function toggleExplorerCollapsed() {
  explorerCollapsed.value = !explorerCollapsed.value
  writeCollapsedState(EXPLORER_COLLAPSED_KEY, explorerCollapsed.value)
}

const workspaceGridStyle = computed(() => {
  if (props.isMobile) return { gridTemplateColumns: '1fr' }
  return {
    gridTemplateColumns: `${explorerCollapsed.value ? SIDEBAR_COLLAPSED_W : SIDEBAR_EXPANDED_W} 1fr`,
  }
})

const collapseBtnClass =
  'flex shrink-0 items-center justify-center rounded w-[22px] h-[22px] text-txt3 transition hover:bg-elevated hover:text-accent-2 focus-visible:shadow-[inset_0_0_0_2px_rgba(99,102,241,0.35)] outline-none'

const filesStep = ref<FilesStep>('list')
const activeFile = ref<DraftFile | null>(null)
const openTabs = ref<DraftFile[]>([])
const expanded = ref<Set<string>>(new Set())
const emptyDirs = ref<Set<string>>(new Set())
const renamingPath = ref('')
const renameInput = ref('')
const creating = ref<{ dir: string; kind: 'file' | 'folder' } | null>(null)
const createInput = ref('')
const folderInput = ref<HTMLInputElement | null>(null)
const uploadTargetDir = ref('')
const selectedTreeRow = ref<TreeRow | null>(null)
const ctxMenu = ref<{ open: boolean; x: number; y: number; target: CtxTarget | null }>({
  open: false,
  x: 0,
  y: 0,
  target: null,
})

type ExplorerMoreTarget = { dir: boolean; path: string; name: string }
const explorerMore = ref<ExplorerMoreTarget | null>(null)
const explorerMoreStyle = ref<Record<string, string>>({})
const explorerMoreAnchor = ref<HTMLElement | null>(null)

type ConfirmCfg = {
  title: string
  message: string
  confirmText?: string
  danger?: boolean
  ok: () => void | Promise<void>
}
const confirmCfg = ref<ConfirmCfg | null>(null)

type LeaveConfirmCfg = {
  title: string
  message: string
  saveText: string
  discardText: string
  onSave: () => boolean | Promise<boolean>
  onDiscard: () => void
}
const leaveConfirmCfg = ref<LeaveConfirmCfg | null>(null)

const currentFile = computed(() => activeFile.value)
const activePath = computed(() => activeFile.value?.path || '')
const breadcrumb = computed(() => activePath.value.split('/').filter(Boolean))

function langForPath(p: string): string {
  const ext = p.split('.').pop()?.toLowerCase() || ''
  return ({ md: 'markdown', markdown: 'markdown', json: 'json', sh: 'shell', bash: 'shell', zsh: 'shell', js: 'javascript', mjs: 'javascript', ts: 'typescript', py: 'python', yml: 'yaml', yaml: 'yaml', toml: 'ini', txt: 'plaintext' } as Record<string, string>)[ext] || 'plaintext'
}

function isMdPath(p: string): boolean {
  return p.toLowerCase().endsWith('.md')
}

function hideCtxMenu() {
  ctxMenu.value.open = false
  ctxMenu.value.target = null
}

function openCtxMenu(e: MouseEvent, target: CtxTarget) {
  e.preventDefault()
  e.stopPropagation()
  selectedTreeRow.value = target.blank || target.dir
    ? null
    : { name: target.name, path: target.path, dir: false, depth: 0 }
  ctxMenu.value = { open: true, x: e.clientX, y: e.clientY, target }
  nextTick(() => {
    const menu = document.querySelector('.explorer-ctx-menu') as HTMLElement | null
    if (!menu) return
    const rect = menu.getBoundingClientRect()
    ctxMenu.value.x = Math.min(e.clientX, window.innerWidth - rect.width - 8)
    ctxMenu.value.y = Math.min(e.clientY, window.innerHeight - rect.height - 8)
  })
}

function onExplorerBlankCtx(e: MouseEvent) {
  openCtxMenu(e, { dir: true, path: '', name: t('pages.agentStudio.configPaths.root'), blank: true })
}

function onCtxAction(action: string) {
  const target = ctxMenu.value.target
  if (!target) return
  const parentDir = target.path.includes('/') ? target.path.slice(0, target.path.lastIndexOf('/')) : ''
  const row: TreeRow = { name: target.name, path: target.path, dir: target.dir, depth: 0 }

  switch (action) {
    case 'newFile':
      newFile(target.blank ? '' : target.dir ? target.path : parentDir)
      break
    case 'newFolder':
      newFolder(target.blank ? '' : target.dir ? target.path : parentDir)
      break
    case 'uploadFolder':
      if (!target.dir || target.blank) return
      uploadTargetDir.value = target.path
      folderInput.value?.click()
      break
    case 'rename':
      if (target.blank) return
      startRename(row)
      break
    case 'copyPath':
      if (target.dir) return
      navigator.clipboard?.writeText(target.path).then(
        () => emit('toast', t('common.toast.pathCopied', { path: target.path })),
        () => emit('toast', t('common.toast.copyPathFailed')),
      )
      break
    case 'delete':
      if (target.path === 'rules' || target.path === 'skills') return
      deleteEntry(row)
      break
  }
  hideCtxMenu()
}

function onExplorerKeydown(e: KeyboardEvent) {
  if (e.key === 'F2' && selectedTreeRow.value && !selectedTreeRow.value.dir) {
    e.preventDefault()
    startRename(selectedTreeRow.value)
  }
  if (e.key === 'Escape') hideCtxMenu()
}

type TreeNode = { name: string; path: string; dir: boolean; children: Record<string, TreeNode> }
type TreeRow = { name: string; path: string; dir: boolean; depth: number }

function joinPath(dir: string, name: string): string {
  return [dir, name].filter(Boolean).join('/').split('/').map((s) => s.trim()).filter(Boolean).join('/')
}

function expandParents(p: string) {
  const segs = p.split('/').filter(Boolean)
  let acc = ''
  for (let i = 0; i < segs.length - 1; i++) {
    acc = acc ? `${acc}/${segs[i]}` : segs[i]
    expanded.value.add(acc)
  }
}

const tree = computed<TreeNode>(() => {
  const root: TreeNode = { name: '', path: '', dir: true, children: {} }
  const add = (path: string, isFile: boolean) => {
    const segs = path.split('/').filter(Boolean)
    let cur = root
    let acc = ''
    segs.forEach((seg, i) => {
      acc = acc ? `${acc}/${seg}` : seg
      const leaf = isFile && i === segs.length - 1
      if (!cur.children[seg]) cur.children[seg] = { name: seg, path: acc, dir: !leaf, children: {} }
      cur = cur.children[seg]
    })
  }
  for (const f of props.draft.files || []) add(f.path, true)
  for (const d of emptyDirs.value) add(d, false)
  return root
})

const rows = computed<TreeRow[]>(() => {
  const out: TreeRow[] = []
  const walk = (node: TreeNode, depth: number) => {
    const kids = Object.values(node.children).sort((a, b) =>
      a.dir === b.dir ? a.name.localeCompare(b.name) : a.dir ? -1 : 1,
    )
    for (const k of kids) {
      out.push({ name: k.name, path: k.path, dir: k.dir, depth })
      if (k.dir && expanded.value.has(k.path)) walk(k, depth + 1)
    }
  }
  walk(tree.value, 0)
  return out
})

function openFile(f: DraftFile) {
  activeFile.value = f
  if (!openTabs.value.includes(f)) openTabs.value.push(f)
  expandParents(f.path)
  selectedTreeRow.value = { name: f.path.split('/').pop() || f.path, path: f.path, dir: false, depth: 0 }
  if (props.isMobile) filesStep.value = 'edit'
}

function openPath(path: string) {
  const f = props.draft.files.find((x) => x.path === path)
  if (f) openFile(f)
}

function selectDefaultFile() {
  if (props.isMobile) {
    activeFile.value = null
    openTabs.value = []
    filesStep.value = 'list'
    return
  }
  const files = [...(props.draft.files || [])].sort((a, b) => a.path.localeCompare(b.path))
  const target = files.find((f) => f.path.toLowerCase().endsWith('.md')) || files[0]
  if (target) openFile(target)
  else activeFile.value = null
}

function resetForSelect() {
  filesStep.value = 'list'
  expanded.value = new Set()
  emptyDirs.value = new Set()
  openTabs.value = []
  activeFile.value = null
  renamingPath.value = ''
  creating.value = null
  closeExplorerMore()
  hideCtxMenu()
  leaveConfirmCfg.value = null
  selectDefaultFile()
}

function snapshot(): FilesOpenSnapshot {
  return {
    path: activeFile.value?.path || '',
    openPaths: openTabs.value.map((f) => f.path),
  }
}

function restoreAfterDiscard(snap: FilesOpenSnapshot) {
  openTabs.value = snap.openPaths
    .map((p) => props.draft.files.find((f) => f.path === p))
    .filter((f): f is DraftFile => !!f)
  activeFile.value = snap.path ? props.draft.files.find((f) => f.path === snap.path) || null : null
}

function goFilesList() {
  filesStep.value = 'list'
}

function tryBackToList() {
  if (!props.dirty) {
    goFilesList()
    return
  }
  leaveConfirmCfg.value = {
    title: t('pages.agentStudio.dialogs.leaveUnsavedTitle'),
    message: t('pages.agentStudio.dialogs.leaveUnsavedBackMessage'),
    saveText: t('pages.agentStudio.dialogs.saveAndBack'),
    discardText: t('pages.agentStudio.dialogs.discardChanges'),
    onSave: async () => {
      const ok = await props.save()
      if (!ok) return false
      emit('update:just-saved', true)
      goFilesList()
      return true
    },
    onDiscard: () => {
      emit('discard')
      goFilesList()
    },
  }
}

async function leaveConfirmSave() {
  const c = leaveConfirmCfg.value
  if (!c) return
  try {
    const ok = await c.onSave()
    if (!ok) return
  } catch (e: any) {
    emit('error', String(e?.message || e))
    return
  }
  leaveConfirmCfg.value = null
}

function leaveConfirmDiscard() {
  const c = leaveConfirmCfg.value
  if (!c) return
  c.onDiscard()
  leaveConfirmCfg.value = null
}

function leaveConfirmCancel() {
  leaveConfirmCfg.value = null
}

function closeTab(f: DraftFile) {
  const i = openTabs.value.indexOf(f)
  if (i < 0) return
  openTabs.value.splice(i, 1)
  if (activeFile.value === f) activeFile.value = openTabs.value[i] || openTabs.value[i - 1] || openTabs.value[openTabs.value.length - 1] || null
}

function startRename(row: TreeRow) {
  renamingPath.value = row.path
  renameInput.value = row.name
  nextTick(() => {
    const el = document.querySelector<HTMLInputElement>('input[data-rename]')
    el?.focus()
    el?.select()
  })
}

function cancelRename() {
  renamingPath.value = ''
}

function commitRename(row: TreeRow) {
  if (renamingPath.value !== row.path) return
  const leaf = renameInput.value.trim().replace(/\//g, '')
  renamingPath.value = ''
  if (!leaf || leaf === row.name) return
  const parent = row.path.includes('/') ? row.path.slice(0, row.path.lastIndexOf('/')) : ''
  const np = joinPath(parent, leaf)
  if (np === row.path) return
  if (props.draft.files.some((f) => f.path === np)) {
    emit('error', t('pages.agentStudio.dialogs.pathExists', { path: np }))
    return
  }
  if (row.dir) {
    const pref = row.path + '/'
    props.draft.files.forEach((f) => {
      if (f.path === row.path || f.path.startsWith(pref)) f.path = np + f.path.slice(row.path.length)
    })
    const ed = new Set<string>()
    expanded.value.forEach((d) => ed.add(d === row.path || d.startsWith(pref) ? np + d.slice(row.path.length) : d))
    expanded.value = ed
    const ned = new Set<string>()
    emptyDirs.value.forEach((d) => ned.add(d === row.path || d.startsWith(pref) ? np + d.slice(row.path.length) : d))
    emptyDirs.value = ned
  } else {
    const f = props.draft.files.find((x) => x.path === row.path)
    if (f) f.path = np
  }
  expandParents(np)
}

function toggleDir(path: string) {
  if (expanded.value.has(path)) expanded.value.delete(path)
  else expanded.value.add(path)
}

function newFile(dir = '') { startCreate('file', dir) }
function newFolder(dir = '') { startCreate('folder', dir) }

function startCreate(kind: 'file' | 'folder', dir = '') {
  renamingPath.value = ''
  emit('error', '')
  createInput.value = ''
  creating.value = { dir, kind }
  if (dir) {
    expandParents(dir)
    expanded.value.add(dir)
  }
  nextTick(() => {
    const el = document.querySelector<HTMLInputElement>('input[data-create]')
    el?.focus()
  })
}

function cancelCreate() {
  creating.value = null
  createInput.value = ''
}

function commitCreate() {
  const c = creating.value
  if (!c) return
  const leaf = createInput.value.trim().replace(/^\/+|\/+$/g, '')
  if (!leaf) { cancelCreate(); return }
  const full = joinPath(c.dir, leaf)
  if (!full) { cancelCreate(); return }
  if (c.kind === 'file') {
    if (props.draft.files.some((f) => f.path === full)) {
      emit('error', t('pages.agentStudio.dialogs.pathExists', { path: full }))
      return
    }
    const f = { path: full, content: '' }
    props.draft.files.push(f)
    emptyDirs.value.delete(c.dir)
    expandParents(full)
    openFile(f)
  } else {
    emptyDirs.value.add(full)
    expandParents(full)
    expanded.value.add(full)
  }
  creating.value = null
  createInput.value = ''
}

function isProtectedDir(path: string) {
  return path === 'rules' || path === 'skills'
}

function deleteEntry(row: TreeRow) {
  if (row.dir && isProtectedDir(row.path)) return
  confirmCfg.value = {
    title: row.dir ? t('pages.agentStudio.dialogs.deleteFolderTitle') : t('pages.agentStudio.dialogs.deleteFileTitle'),
    message: row.dir ? t('pages.agentStudio.dialogs.deleteFolderMessage', { path: row.path }) : t('pages.agentStudio.dialogs.deleteFileMessage', { path: row.path }),
    confirmText: t('pages.agentStudio.dialogs.delete'),
    danger: true,
    ok: () => {
      const gone = row.dir
        ? props.draft.files.filter((f) => f.path === row.path || f.path.startsWith(row.path + '/'))
        : props.draft.files.filter((f) => f.path === row.path)
      props.draft.files = props.draft.files.filter((f) => !gone.includes(f))
      openTabs.value = openTabs.value.filter((f) => !gone.includes(f))
      if (activeFile.value && gone.includes(activeFile.value)) activeFile.value = openTabs.value[openTabs.value.length - 1] || null
      if (row.dir) {
        const pref = row.path + '/'
        const ed = new Set<string>()
        emptyDirs.value.forEach((d) => { if (!(d === row.path || d.startsWith(pref))) ed.add(d) })
        emptyDirs.value = ed
      }
      if (!activeFile.value) selectDefaultFile()
    },
  }
}

async function confirmOk() {
  const c = confirmCfg.value
  if (!c) return
  confirmCfg.value = null
  await c.ok()
}

async function onFolderPick(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  const targetDir = uploadTargetDir.value
  uploadTargetDir.value = ''
  let added = 0
  let first = ''
  for (const f of files) {
    if (f.size > 512 * 1024) continue
    const parts = (f.webkitRelativePath || f.name).split('/').map((s) => s.trim()).filter(Boolean)
    const rel = parts.slice(1).join('/') || parts.join('/')
    if (!rel) continue
    const path = targetDir ? joinPath(targetDir, rel) : rel
    try {
      const content = await f.text()
      const at = props.draft.files.findIndex((x) => x.path === path)
      if (at >= 0) props.draft.files[at].content = content
      else props.draft.files.push({ path, content })
      if (!first) first = path
      added++
    } catch {
      /* skip */
    }
  }
  if (!added) {
    emit('error', t('pages.agentStudio.dialogs.importNoText'))
    return
  }
  props.draft.files.sort((a, b) => a.path.localeCompare(b.path))
  if (first) openPath(first)
  emit('toast', t('common.toast.importedFiles', { count: added, dir: targetDir ? targetDir + '/' : t('pages.agentStudio.configPaths.root') }))
}

function closeExplorerMore() {
  explorerMore.value = null
  explorerMoreAnchor.value = null
}

function explorerMoreItemCount(target: ExplorerMoreTarget) {
  if (target.dir) return isProtectedDir(target.path) ? 3 : 4
  return 2
}

function placeExplorerMore() {
  const anchor = explorerMoreAnchor.value
  const target = explorerMore.value
  if (!anchor || !target) return
  const rect = anchor.getBoundingClientRect()
  const margin = 8
  const menuWidth = Math.min(220, Math.max(160, window.innerWidth - margin * 2))
  const menuHeight = explorerMoreItemCount(target) * 44 + 8
  let left = rect.right - menuWidth
  left = Math.max(margin, Math.min(left, window.innerWidth - margin - menuWidth))
  let top = rect.bottom + 4
  if (top + menuHeight > window.innerHeight - margin) {
    top = Math.max(margin, rect.top - menuHeight - 4)
  }
  explorerMoreStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${Math.round(menuWidth)}px`,
  }
}

function toggleExplorerMore(e: MouseEvent, row: TreeRow) {
  const el = e.currentTarget as HTMLElement
  if (explorerMore.value?.path === row.path) {
    closeExplorerMore()
    return
  }
  hideCtxMenu()
  explorerMore.value = { dir: row.dir, path: row.path, name: row.name }
  explorerMoreAnchor.value = el
  nextTick(placeExplorerMore)
}

function onExplorerMoreAction(action: 'newFile' | 'newFolder' | 'rename' | 'delete') {
  const target = explorerMore.value
  if (!target) return
  const row: TreeRow = { name: target.name, path: target.path, dir: target.dir, depth: 0 }
  closeExplorerMore()
  if (action === 'newFile' && target.dir) newFile(target.path)
  else if (action === 'newFolder' && target.dir) newFolder(target.path)
  else if (action === 'rename') startRename(row)
  else if (action === 'delete' && !(target.dir && isProtectedDir(target.path))) deleteEntry(row)
}

function onDocumentClick() {
  hideCtxMenu()
}

function onChromeReposition() {
  if (explorerMore.value) placeExplorerMore()
}

function onChromeKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && explorerMore.value) closeExplorerMore()
}

watch(
  () => props.isMobile,
  (mobile) => {
    if (mobile) {
      filesStep.value = activeFile.value ? 'edit' : 'list'
    } else {
      closeExplorerMore()
    }
  },
)

onMounted(() => {
  explorerCollapsed.value = readCollapsedState(EXPLORER_COLLAPSED_KEY)
  document.addEventListener('click', onDocumentClick)
  document.addEventListener('keydown', onExplorerKeydown)
  document.addEventListener('keydown', onChromeKeydown)
  window.addEventListener('resize', onChromeReposition)
  window.addEventListener('scroll', onChromeReposition, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick)
  document.removeEventListener('keydown', onExplorerKeydown)
  document.removeEventListener('keydown', onChromeKeydown)
  window.removeEventListener('resize', onChromeReposition)
  window.removeEventListener('scroll', onChromeReposition, true)
})

defineExpose({
  filesStep,
  resetForSelect,
  snapshot,
  restoreAfterDiscard,
  closeExplorerMore,
  selectDefaultFile,
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
  <div
    class="min-h-0 flex-1 overflow-hidden transition-[grid-template-columns] duration-[220ms] ease-in-out"
    :class="isMobile ? 'flex flex-col' : 'grid'"
    :style="isMobile ? undefined : workspaceGridStyle"
  >
        <!-- explorer (list step on mobile) -->
        <div
          v-if="!isMobile || filesStep === 'list'"
          class="flex min-h-0 min-w-0 flex-col bg-base/30"
          :class="isMobile ? 'flex-1' : 'border-r border-line'"
        >
          <div
            class="flex shrink-0 items-center gap-0.5 border-b border-line px-2 py-1.5"
            :class="[
              isMobile ? 'min-h-11' : 'min-h-8',
              explorerCollapsed && !isMobile ? 'justify-center px-[3px]' : '',
            ]"
          >
            <span
              v-if="!explorerCollapsed || isMobile"
              class="flex-1 truncate px-1 text-[10.5px] font-semibold uppercase tracking-wider text-txt3"
            >{{ t('pages.agentStudio.explorer.title') }}</span>
            <template v-if="!explorerCollapsed || isMobile">
              <button
                class="rounded p-1 text-txt3 hover:bg-elevated hover:text-accent-2 focus-visible:shadow-[inset_0_0_0_2px_rgba(99,102,241,0.35)] outline-none"
                :class="isMobile ? 'min-h-11 min-w-11' : ''"
                :title="t('pages.agentStudio.explorer.newFile')"
                @click="newFile('')"
              ><Icon name="doc" :size="13" /></button>
              <button
                class="rounded p-1 text-txt3 hover:bg-elevated hover:text-accent-2 focus-visible:shadow-[inset_0_0_0_2px_rgba(99,102,241,0.35)] outline-none"
                :class="isMobile ? 'min-h-11 min-w-11' : ''"
                :title="t('pages.agentStudio.explorer.newFolder')"
                @click="newFolder('')"
              ><Icon name="folder" :size="13" /></button>
              <input ref="folderInput" type="file" webkitdirectory directory multiple class="hidden" @change="onFolderPick" />
              <button
                v-if="!isMobile"
                type="button"
                :class="collapseBtnClass"
                :title="t('pages.agentStudio.explorer.collapse')"
                :aria-label="t('pages.agentStudio.explorer.collapse')"
                @click="toggleExplorerCollapsed"
              >
                <Icon name="chevron-right" :size="14" class="rotate-180" />
              </button>
            </template>
            <button
              v-if="explorerCollapsed && !isMobile"
              type="button"
              :class="collapseBtnClass"
              :title="t('pages.agentStudio.explorer.expand')"
              :aria-label="t('pages.agentStudio.explorer.expand')"
              @click="toggleExplorerCollapsed"
            >
              <Icon name="chevron-right" :size="14" />
            </button>
          </div>
          <div
            v-if="!explorerCollapsed || isMobile"
            class="scroll-area min-h-0 flex-1 overflow-y-auto py-1"
            @contextmenu="onExplorerBlankCtx"
          >
            <div v-if="!rows.length && !creating" class="px-3 py-6 text-center text-[11px] leading-5 text-txt3">{{ t('pages.agentStudio.explorer.empty') }}</div>

            <!-- inline new entry at root (VSCode-style) -->
            <div v-if="creating && creating.dir === ''" class="flex w-full items-center gap-1 py-[3px] pr-1.5">
              <span class="flex shrink-0" :style="{ width: '8px' }" />
              <Icon :name="creating.kind === 'folder' ? 'folder' : 'doc'" :size="13" class="shrink-0" :class="creating.kind === 'folder' ? 'text-accent-2' : 'text-txt3'" />
              <input
                data-create
                v-model="createInput"
                class="min-w-0 flex-1 rounded border border-accent bg-surface px-1 py-0 font-mono text-[12px] text-txt outline-none"
                :placeholder="creating.kind === 'folder' ? t('pages.agentStudio.explorer.folderPlaceholder') : t('pages.agentStudio.explorer.filePlaceholder')"
                @keyup.enter="commitCreate"
                @keyup.esc="cancelCreate"
                @blur="commitCreate"
                @click.stop
              />
            </div>

            <template v-for="row in rows" :key="(row.dir ? 'd:' : 'f:') + row.path">
            <div
              class="group relative flex w-full items-center gap-1 pr-1.5 text-left text-[12px] transition"
              :class="[
                isMobile ? 'min-h-11 py-2' : 'py-[3px]',
                !row.dir && activePath === row.path ? 'bg-accent-dim text-txt' : 'text-txt2 hover:bg-elevated',
                ctxMenu.target?.path === row.path ? 'bg-overlay outline outline-1 outline-accent/35' : '',
              ]"
              @dblclick="startRename(row)"
              @contextmenu="openCtxMenu($event, { dir: row.dir, path: row.path, name: row.name })"
            >
              <span
                v-if="!row.dir && activePath === row.path"
                class="absolute inset-y-0 left-0 w-0.5 bg-accent"
              />
              <!-- indent guides -->
              <span class="flex shrink-0" :style="{ width: 8 + row.depth * 12 + 'px' }">
                <span v-for="d in row.depth" :key="d" class="ml-[5px] w-[7px] border-l border-line/60" />
              </span>

              <template v-if="renamingPath === row.path">
                <Icon :name="row.dir ? 'folder' : 'doc'" :size="13" class="shrink-0" :class="row.dir ? 'text-accent-2' : 'text-txt3'" />
                <input
                  data-rename
                  v-model="renameInput"
                  class="min-w-0 flex-1 rounded border border-accent bg-surface px-1 py-0 font-mono text-[12px] text-txt outline-none"
                  @keyup.enter="commitRename(row)"
                  @keyup.esc="cancelRename"
                  @blur="commitRename(row)"
                  @click.stop
                />
              </template>

              <template v-else>
                <button v-if="row.dir" class="flex min-w-0 flex-1 items-center gap-1" @click="toggleDir(row.path)">
                  <Icon :name="expanded.has(row.path) ? 'chevron-down' : 'chevron-right'" :size="12" class="shrink-0 text-txt3" />
                  <Icon name="folder" :size="13" class="shrink-0 text-accent-2" />
                  <span class="truncate font-mono">{{ row.name }}</span>
                </button>
                <button v-else class="flex min-w-0 flex-1 items-center gap-1 pl-[15px]" @click="openPath(row.path); selectedTreeRow = row">
                  <Icon name="doc" :size="13" class="shrink-0 text-txt3" />
                  <span class="truncate font-mono">{{ row.name }}</span>
                </button>
                <button
                  v-if="isMobile"
                  type="button"
                  data-test="file-row-more"
                  :data-path="row.path"
                  class="flex min-h-11 min-w-11 shrink-0 items-center justify-center text-txt3 hover:text-accent-2"
                  :title="t('pages.agentStudio.explorer.more')"
                  :aria-label="t('pages.agentStudio.explorer.more')"
                  :aria-expanded="explorerMore?.path === row.path"
                  @click.stop="toggleExplorerMore($event, row)"
                ><Icon name="more" :size="16" /></button>
                <template v-else>
                  <button
                    v-if="row.dir"
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 opacity-0 hover:text-accent-2 group-hover:opacity-100"
                    :title="t('pages.agentStudio.explorer.newFileInFolder')"
                    @click.stop="newFile(row.path)"
                  ><Icon name="doc" :size="12" /></button>
                  <button
                    v-if="row.dir"
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 opacity-0 hover:text-accent-2 group-hover:opacity-100"
                    :title="t('pages.agentStudio.explorer.newFolderInFolder')"
                    @click.stop="newFolder(row.path)"
                  ><Icon name="folder" :size="12" /></button>
                  <button
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 opacity-0 hover:text-accent-2 group-hover:opacity-100"
                    :title="t('pages.agentStudio.explorer.rename')"
                    @click.stop="startRename(row)"
                  ><Icon name="edit" :size="12" /></button>
                  <button
                    v-if="!row.dir || !isProtectedDir(row.path)"
                    data-test="file-row-action"
                    class="shrink-0 text-txt3 opacity-0 hover:text-err group-hover:opacity-100"
                    :title="t('pages.agentStudio.explorer.delete')"
                    @click.stop="deleteEntry(row)"
                  ><Icon name="close" :size="12" /></button>
                </template>
              </template>
            </div>

            <!-- inline new entry under this folder (VSCode-style) -->
            <div v-if="creating && creating.dir === row.path" class="flex w-full items-center gap-1 py-[3px] pr-1.5">
              <span class="flex shrink-0" :style="{ width: 8 + (row.depth + 1) * 12 + 'px' }">
                <span v-for="d in (row.depth + 1)" :key="d" class="ml-[5px] w-[7px] border-l border-line/60" />
              </span>
              <Icon :name="creating.kind === 'folder' ? 'folder' : 'doc'" :size="13" class="shrink-0" :class="creating.kind === 'folder' ? 'text-accent-2' : 'text-txt3'" />
              <input
                data-create
                v-model="createInput"
                class="min-w-0 flex-1 rounded border border-accent bg-surface px-1 py-0 font-mono text-[12px] text-txt outline-none"
                :placeholder="creating.kind === 'folder' ? t('pages.agentStudio.explorer.folderName') : t('pages.agentStudio.explorer.fileName')"
                @keyup.enter="commitCreate"
                @keyup.esc="cancelCreate"
                @blur="commitCreate"
                @click.stop
              />
            </div>
            </template>
          </div>
        </div>

        <!-- editor pane (edit step on mobile) -->
        <div
          v-if="!isMobile || filesStep === 'edit'"
          class="flex min-h-0 min-w-0 flex-col overflow-hidden"
          :class="isMobile ? 'flex-1' : ''"
        >
          <!-- mobile edit chrome: back + path -->
          <div
            v-if="isMobile"
            class="flex shrink-0 items-center gap-2 border-b border-line bg-base/25 px-2.5 py-2 min-h-12"
          >
            <button
              type="button"
              class="inline-flex min-h-11 items-center gap-1 px-2 text-[12px] text-txt2 hover:text-txt"
              @click="tryBackToList"
            >
              <Icon name="chevron-right" :size="14" class="rotate-180" />
              {{ t('pages.agentStudio.mobile.back') }}
            </button>
            <span class="min-w-0 flex-1 truncate font-mono text-[12px] text-txt">{{ activePath }}</span>
          </div>

          <!-- open-file tabs (desktop only; mobile uses single-file full-screen edit) -->
          <div
            v-if="!isMobile && openTabs.length"
            class="scroll-area flex shrink-0 items-stretch overflow-x-auto border-b border-line bg-base/40"
          >
            <div
              v-for="tabFile in openTabs"
              :key="tabFile.path"
              class="group flex shrink-0 cursor-pointer items-center gap-1.5 border-r border-line px-3 py-1.5 text-[12px] transition"
              :class="activeFile === tabFile ? 'bg-surface text-txt' : 'text-txt3 hover:bg-elevated'"
              @click="activeFile = tabFile"
            >
              <Icon name="doc" :size="12" class="shrink-0" />
              <span class="max-w-[160px] truncate font-mono">{{ tabFile.path.split('/').pop() }}</span>
              <button class="shrink-0 rounded text-txt3 opacity-0 hover:bg-overlay hover:text-txt group-hover:opacity-100" :class="activeFile === tabFile ? 'opacity-60' : ''" :title="t('pages.agentStudio.explorer.close')" @click.stop="closeTab(tabFile)"><Icon name="close" :size="12" /></button>
            </div>
          </div>

          <template v-if="currentFile">
            <!-- breadcrumb (desktop) -->
            <div
              v-if="!isMobile"
              class="flex shrink-0 items-center gap-1 border-b border-line px-3 py-1 text-[11px] text-txt3"
            >
              <Icon name="folder" :size="11" class="text-accent-2/70" />
              <template v-for="(seg, i) in breadcrumb" :key="i">
                <Icon v-if="i > 0" name="chevron-right" :size="10" class="text-txt3/60" />
                <span :class="i === breadcrumb.length - 1 ? 'font-mono text-txt2' : 'font-mono'">{{ seg }}</span>
              </template>
            </div>
            <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
              <MarkdownSplitEditor
                v-if="isMdPath(currentFile.path)"
                v-model="currentFile.content"
                :file-path="currentFile.path"
                :variant="isMobile ? 'stack' : 'split'"
              />
              <CodeEditor v-else v-model="currentFile.content" :language="langForPath(currentFile.path)" />
            </div>
            <!-- status bar -->
            <div
              class="flex shrink-0 items-center gap-3 border-t border-line bg-base/40 px-3 py-1 text-[10.5px] text-txt3"
            >
              <span class="uppercase">{{ langForPath(currentFile.path) }}</span>
              <span>{{ t('common.format.lines', { n: currentFile.content.split('\n').length }) }}</span>
              <span>{{ t('common.format.chars', { n: currentFile.content.length }) }}</span>
              <span
                v-if="!isMobile && isMdPath(currentFile.path)"
                class="border border-line bg-elevated px-1.5 text-[10px] text-txt2"
              >{{ t('pages.agentStudio.explorer.markdownBadge') }}</span>
              <span class="ml-auto truncate font-mono">{{ currentFile.path }}</span>
            </div>
          </template>
          <div v-else class="flex flex-1 flex-col items-center justify-center gap-2 text-sm text-txt3">
            <Icon name="doc" :size="28" class="text-line-strong" />
            {{ t('pages.agentStudio.explorer.selectOrCreate') }}
          </div>
        </div>
  </div>

    <AppModal
      :open="!!leaveConfirmCfg"
      :title="leaveConfirmCfg?.title || ''"
      :width="420"
      @close="leaveConfirmCancel"
    >
      <p class="text-[13px] leading-6 text-txt2">{{ leaveConfirmCfg?.message }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="leaveConfirmCancel">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="outline" @click="leaveConfirmDiscard">{{ leaveConfirmCfg?.discardText }}</AppButton>
        <AppButton size="sm" variant="primary" @click="leaveConfirmSave">{{ leaveConfirmCfg?.saveText }}</AppButton>
      </template>
    </AppModal>

    <AppModal
      :open="!!confirmCfg"
      :title="confirmCfg?.title || ''"
      :width="420"
      @close="confirmCfg = null"
    >
      <p class="text-[13px] leading-6 text-txt2">{{ confirmCfg?.message }}</p>
      <template #footer>
        <AppButton size="sm" variant="ghost" @click="confirmCfg = null">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" :variant="confirmCfg?.danger ? 'danger' : 'primary'" @click="confirmOk">{{ confirmCfg?.confirmText || t('common.buttons.confirm') }}</AppButton>
      </template>
    </AppModal>

    <Teleport to="body">
      <div
        v-if="isMobile && explorerMore"
        data-test="explorer-more-backdrop"
        class="fixed inset-0 z-[9998]"
        @click="closeExplorerMore"
      />
      <div
        v-if="isMobile && explorerMore"
        data-test="explorer-more-menu"
        class="fixed z-[9999] min-w-[180px] border border-line bg-elevated py-1 shadow-card"
        :style="explorerMoreStyle"
        @click.stop
      >
        <button
          v-if="explorerMore.dir"
          type="button"
          data-test="explorer-more-item"
          data-action="newFile"
          class="flex min-h-11 w-full items-center px-3.5 text-left text-[13px] text-txt hover:bg-overlay"
          @click="onExplorerMoreAction('newFile')"
        >{{ t('pages.agentStudio.explorer.newFile') }}</button>
        <button
          v-if="explorerMore.dir"
          type="button"
          data-test="explorer-more-item"
          data-action="newFolder"
          class="flex min-h-11 w-full items-center px-3.5 text-left text-[13px] text-txt hover:bg-overlay"
          @click="onExplorerMoreAction('newFolder')"
        >{{ t('pages.agentStudio.explorer.newFolder') }}</button>
        <button
          type="button"
          data-test="explorer-more-item"
          data-action="rename"
          class="flex min-h-11 w-full items-center px-3.5 text-left text-[13px] text-txt hover:bg-overlay"
          @click="onExplorerMoreAction('rename')"
        >{{ t('pages.agentStudio.explorer.rename') }}</button>
        <button
          v-if="!explorerMore.dir || !isProtectedDir(explorerMore.path)"
          type="button"
          data-test="explorer-more-item"
          data-action="delete"
          class="flex min-h-11 w-full items-center px-3.5 text-left text-[13px] text-err hover:bg-err/10"
          @click="onExplorerMoreAction('delete')"
        >{{ t('pages.agentStudio.explorer.delete') }}</button>
      </div>
    </Teleport>

    <ExplorerContextMenu
      :open="ctxMenu.open"
      :x="ctxMenu.x"
      :y="ctxMenu.y"
      :target="ctxMenu.target"
      @close="hideCtxMenu"
      @action="onCtxAction"
    />
  </div>
</template>

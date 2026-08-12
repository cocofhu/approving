<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api } from '@/lib/api/api'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import { useToast } from '@/lib/composables/useToast'
import {
  clampDraftSplitRatio,
  findNextDraftMatch,
  highlightDraftSource,
  insertDraftMarkdown,
  syncScrollTop,
  type DraftInsertCmd,
} from '@/lib/project/requirementDraftMarkdown'
import {
  barStyle,
  buildGanttRows,
  clampProgress,
  computeTimelineWindow,
  diamondStyle,
  draftBarRange,
  draftHasChildren,
  normalizeDraft,
  parentCandidates,
  sortMilestones,
  tickLabel,
  todayLocalISO,
  withContextualParents,
  type GanttRow,
  type GanttScale,
} from '@/lib/project/requirementDraftSchedule'
import { fmtTime } from '@/lib/shared/format'
import { renderMarkdown } from '@/lib/shared/markdown'
import type {
  RequirementDraft,
  RequirementDraftKind,
  RequirementDraftSchedulePatch,
  RequirementDraftStatusFilter,
  RequirementDraftViewMode,
} from '@/lib/shared/types'

const props = defineProps<{
  projectId: string
}>()

const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()

const viewMode = ref<RequirementDraftViewMode>('gantt')
const filter = ref<RequirementDraftStatusFilter>('open')
const query = ref('')
const items = ref<RequirementDraft[]>([])
const catalog = ref<RequirementDraft[]>([])
const loading = ref(false)
const selectedId = ref<string | null>(null)
const creating = ref(false)
const saving = ref(false)
const statusBusy = ref(false)
const deleting = ref(false)
const scheduleBusy = ref(false)
const showDelete = ref(false)
const showLeave = ref(false)
const showNewModal = ref(false)

const newModalKind = ref<RequirementDraftKind>('requirement')
const newModalDueAt = ref('')
const newModalError = ref('')

const ganttScale = ref<GanttScale>('week')

const editTitle = ref('')
const editBody = ref('')
const savedTitle = ref('')
const savedBody = ref('')
const titleError = ref('')
const scheduleError = ref('')
const editKind = ref<RequirementDraftKind>('requirement')
const editStartAt = ref('')
const editDueAt = ref('')
const editProgress = ref(0)
const editParentId = ref<string | null>(null)
const createdAtLabel = ref('—')
const updatedAtLabel = ref('—')
const selectedStatus = ref<'open' | 'done'>('open')

const previewHtml = ref('')
const highlightHtml = ref(' ')
const findOpen = ref(false)
const findQuery = ref('')
const previewCollapsed = ref(false)
const mobilePane = ref<'src' | 'prev'>('src')
const splitRatio = ref(0.5)
const sashDragging = ref(false)

const srcEl = ref<HTMLTextAreaElement | null>(null)
const hlEl = ref<HTMLElement | null>(null)
const gutterEl = ref<HTMLElement | null>(null)
const previewEl = ref<HTMLElement | null>(null)
const splitEl = ref<HTMLElement | null>(null)
const findInputEl = ref<HTMLInputElement | null>(null)

const hasSelection = computed(() => Boolean(selectedId.value))
const isDirty = computed(
  () => editTitle.value !== savedTitle.value || editBody.value !== savedBody.value,
)
const lineNumbers = computed(() => {
  const n = Math.max(1, String(editBody.value || '').split('\n').length)
  return Array.from({ length: n }, (_, i) => i + 1)
})
const showSplitPreview = computed(() => {
  if (isMobile.value) return mobilePane.value === 'prev'
  return !previewCollapsed.value
})
const showSplitSource = computed(() => {
  if (isMobile.value) return mobilePane.value === 'src'
  return true
})
const sourceWidthStyle = computed(() => {
  if (isMobile.value || previewCollapsed.value || !showSplitPreview.value) {
    return { width: '100%' }
  }
  return { width: `${splitRatio.value * 100}%` }
})

const catalogById = computed(() => new Map(catalog.value.map((d) => [d.id, d])))

const contextualDisplay = computed(() => {
  const matched = items.value
  if (!query.value.trim()) {
    return matched.map((draft) => ({ draft, contextual: false }))
  }
  return withContextualParents(matched, catalogById.value)
})

const contextualIds = computed(
  () => new Set(contextualDisplay.value.filter((x) => x.contextual).map((x) => x.draft.id)),
)

const ganttRows = computed(() =>
  buildGanttRows(
    contextualDisplay.value.map((x) => x.draft),
    { contextualIds: contextualIds.value },
  ),
)

const timelineDrafts = computed(() => {
  const seen = new Set<string>()
  const out: RequirementDraft[] = []
  for (const row of [...ganttRows.value.unscheduled, ...ganttRows.value.scheduled]) {
    if (seen.has(row.draft.id)) continue
    if (draftBarRange(row.draft)) {
      seen.add(row.draft.id)
      out.push(row.draft)
    }
  }
  return out
})

const timelineWindow = computed(() => computeTimelineWindow(timelineDrafts.value, ganttScale.value))

const todayLineStyle = computed(() => {
  const today = todayLocalISO()
  return { left: barStyle(today, today, timelineWindow.value).left }
})

const milestoneItems = computed(() => sortMilestones(items.value))

const selectedDraft = computed(() => {
  if (!selectedId.value) return null
  return (
    items.value.find((d) => d.id === selectedId.value) ||
    catalog.value.find((d) => d.id === selectedId.value) ||
    null
  )
})

const parentOptions = computed(() => {
  const id = selectedId.value
  if (!id) return []
  return parentCandidates(catalog.value, id)
})

const canEditParent = computed(() => {
  const id = selectedId.value
  if (!id) return false
  return !draftHasChildren(catalog.value, id)
})

let leaveWaiter: ((ok: boolean) => void) | null = null
let loadSeq = 0
let previewRaf = 0
let highlightRaf = 0
let scrollLock = false

function emptyPreviewHtml() {
  return `<p class="text-txt3">${t('pages.projectDetail.requirementDrafts.emptyBodyPreview')}</p>`
}

function schedulePreview() {
  if (previewRaf) cancelAnimationFrame(previewRaf)
  previewRaf = requestAnimationFrame(() => {
    const src = editBody.value
    previewHtml.value = !String(src || '').trim() ? emptyPreviewHtml() : renderMarkdown(src)
  })
}

function scheduleHighlight() {
  if (highlightRaf) cancelAnimationFrame(highlightRaf)
  highlightRaf = requestAnimationFrame(() => {
    highlightHtml.value = highlightDraftSource(editBody.value)
  })
}

function applySavedBaseline(title: string, body: string) {
  savedTitle.value = title
  savedBody.value = body
}

function discardLocalBuffer() {
  editTitle.value = savedTitle.value
  editBody.value = savedBody.value
  titleError.value = ''
}

/** Restore schedule form fields from a draft without touching scheduleError. */
function applyScheduleFieldsFromDraft(d: RequirementDraft) {
  const n = normalizeDraft(d)
  editKind.value = n.kind
  editStartAt.value = n.startAt
  editDueAt.value = n.dueAt
  editProgress.value = n.progress
  editParentId.value = n.parentId
}

function applyScheduleFromDraft(d: RequirementDraft) {
  applyScheduleFieldsFromDraft(d)
  scheduleError.value = ''
}

function mergeScheduleIntoItems(updated: RequirementDraft) {
  const norm = normalizeDraft(updated)
  const patch = (list: RequirementDraft[]) => {
    const i = list.findIndex((d) => d.id === norm.id)
    if (i >= 0) list[i] = norm
  }
  patch(items.value)
  patch(catalog.value)
}

function mapScheduleApiError(msg: string): string {
  const m = String(msg || '').toLowerCase()
  if (m.includes('due') && m.includes('before') && m.includes('start')) {
    return t('pages.projectDetail.requirementDrafts.errDueBeforeStart')
  }
  if (m.includes('invalid date') || m.includes('yyyy-mm-dd')) {
    return t('pages.projectDetail.requirementDrafts.errInvalidDate')
  }
  if (m.includes('milestone') && (m.includes('due') || m.includes('date'))) {
    return t('pages.projectDetail.requirementDrafts.errMilestoneDue')
  }
  if (m.includes('parent')) return t('pages.projectDetail.requirementDrafts.errInvalidParent')
  if (m.includes('children') || m.includes('child')) {
    return t('pages.projectDetail.requirementDrafts.errHasChildren')
  }
  if (m.includes('kind') && m.includes('date')) {
    return t('pages.projectDetail.requirementDrafts.errKindNeedsDate')
  }
  if (m.includes('progress')) return t('pages.projectDetail.requirementDrafts.errInvalidProgress')
  if (m.includes('kind')) return t('pages.projectDetail.requirementDrafts.errInvalidKind')
  return msg || t('pages.projectDetail.requirementDrafts.scheduleFailed')
}

function revertScheduleFields() {
  const d = selectedDraft.value
  // Keep scheduleError so near-field inline hints survive failed PATCH (NFR n2 / g3.3).
  if (d) applyScheduleFieldsFromDraft(d)
}

function requestLeave(): Promise<boolean> {
  if (!isDirty.value) return Promise.resolve(true)
  if (leaveWaiter) {
    return new Promise((resolve) => {
      const prev = leaveWaiter
      leaveWaiter = (ok) => {
        prev?.(ok)
        resolve(ok)
      }
    })
  }
  showLeave.value = true
  return new Promise((resolve) => {
    leaveWaiter = resolve
  })
}

function resolveLeave(ok: boolean) {
  showLeave.value = false
  if (ok) discardLocalBuffer()
  const done = leaveWaiter
  leaveWaiter = null
  done?.(ok)
}

async function loadCatalog() {
  const res = await api.listRequirementDrafts(props.projectId, { status: filter.value })
  catalog.value = (Array.isArray(res.items) ? res.items : []).map(normalizeDraft)
}

async function loadList(opts?: { preferId?: string | null; keepSelection?: boolean }) {
  const seq = ++loadSeq
  loading.value = true
  try {
    const [res] = await Promise.all([
      api.listRequirementDrafts(props.projectId, {
        status: filter.value,
        q: query.value.trim() || undefined,
      }),
      loadCatalog(),
    ])
    if (seq !== loadSeq) return
    items.value = (Array.isArray(res.items) ? res.items : []).map(normalizeDraft)
    const prefer = opts?.preferId
    if (prefer && items.value.some((d) => d.id === prefer)) {
      selectDraft(prefer, { discardBuffer: opts?.keepSelection ? false : true })
      return
    }
    if (opts?.keepSelection && selectedId.value) {
      const cur =
        items.value.find((d) => d.id === selectedId.value) ||
        catalog.value.find((d) => d.id === selectedId.value)
      if (cur) {
        createdAtLabel.value = fmtTime(cur.createdAt)
        updatedAtLabel.value = fmtTime(cur.updatedAt)
        selectedStatus.value = cur.status
        applyScheduleFromDraft(cur)
      }
      return
    }
    if (selectedId.value) {
      const inItems = items.value.some((d) => d.id === selectedId.value)
      const inCatalog = catalog.value.some((d) => d.id === selectedId.value)
      if (inItems || inCatalog) {
        selectDraft(selectedId.value, { discardBuffer: false })
        return
      }
    }
    selectedId.value = null
  } catch (e: any) {
    if (seq !== loadSeq) return
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.loadFailed'))
    items.value = []
    catalog.value = []
    selectedId.value = null
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

function selectDraft(id: string, opts?: { discardBuffer?: boolean }) {
  const d =
    items.value.find((x) => x.id === id) || catalog.value.find((x) => x.id === id)
  if (!d) {
    selectedId.value = null
    return
  }
  selectedId.value = id
  if (opts?.discardBuffer !== false) {
    editTitle.value = d.title
    editBody.value = d.bodyMarkdown || ''
    titleError.value = ''
    applySavedBaseline(d.title, d.bodyMarkdown || '')
  }
  applyScheduleFromDraft(d)
  createdAtLabel.value = fmtTime(d.createdAt)
  updatedAtLabel.value = fmtTime(d.updatedAt)
  selectedStatus.value = d.status
}

async function onPickDraft(id: string) {
  if (id === selectedId.value) return
  if (isDirty.value) {
    const ok = await requestLeave()
    if (!ok) return
  }
  selectDraft(id, { discardBuffer: true })
}

async function onSelectFromGantt(id: string) {
  await onPickDraft(id)
}

function setViewMode(mode: RequirementDraftViewMode) {
  viewMode.value = mode
}

function setGanttScale(scale: GanttScale) {
  ganttScale.value = scale
}

function openNewModal() {
  newModalKind.value = 'requirement'
  newModalDueAt.value = ''
  newModalError.value = ''
  showNewModal.value = true
}

function closeNewModal() {
  showNewModal.value = false
  newModalError.value = ''
}

async function onNewClick() {
  if (creating.value) return
  if (isDirty.value) {
    const ok = await requestLeave()
    if (!ok) return
  }
  openNewModal()
}

async function onConfirmCreate() {
  newModalError.value = ''
  if (newModalKind.value === 'milestone' && !newModalDueAt.value.trim()) {
    newModalError.value = t('pages.projectDetail.requirementDrafts.milestoneDueRequired')
    return
  }
  creating.value = true
  try {
    const body =
      newModalKind.value === 'milestone'
        ? { kind: 'milestone' as const, dueAt: newModalDueAt.value.trim() }
        : { kind: 'requirement' as const }
    const created = await api.createRequirementDraft(props.projectId, body)
    closeNewModal()
    filter.value = 'open'
    query.value = ''
    const norm = normalizeDraft(created)
    if (newModalKind.value === 'requirement' && viewMode.value === 'milestones') {
      viewMode.value = 'edit'
    } else if (newModalKind.value === 'requirement') {
      viewMode.value = 'edit'
    }
    await loadList({ preferId: norm.id })
    toast.success(t('pages.projectDetail.requirementDrafts.createdToast'))
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.createFailed'))
  } finally {
    creating.value = false
  }
}

async function patchSchedule(body: RequirementDraftSchedulePatch) {
  const id = selectedId.value
  if (!id || scheduleBusy.value) return
  scheduleBusy.value = true
  scheduleError.value = ''
  try {
    const updated = await api.patchRequirementDraftSchedule(props.projectId, id, body)
    mergeScheduleIntoItems(updated)
    applyScheduleFromDraft(updated)
    await loadList({ keepSelection: true })
    toast.success(t('pages.projectDetail.requirementDrafts.scheduleSaved'))
  } catch (e: any) {
    // Revert inputs first, then set error (revert must not clear scheduleError).
    revertScheduleFields()
    scheduleError.value = mapScheduleApiError(e?.message || '')
    toast.error(scheduleError.value)
  } finally {
    scheduleBusy.value = false
  }
}

function onScheduleKindChange() {
  const kind = editKind.value
  void patchSchedule({ kind })
}

function onScheduleStartChange() {
  void patchSchedule({ startAt: editStartAt.value })
}

function onScheduleDueChange() {
  void patchSchedule({ dueAt: editDueAt.value })
}

function onScheduleProgressChange() {
  editProgress.value = clampProgress(editProgress.value)
  void patchSchedule({ progress: editProgress.value })
}

function onScheduleParentChange() {
  void patchSchedule({ parentId: editParentId.value })
}

async function openBody(id?: string) {
  const targetId = id || selectedId.value
  if (!targetId) return
  if (targetId !== selectedId.value && isDirty.value) {
    const ok = await requestLeave()
    if (!ok) return
  }
  if (targetId !== selectedId.value) {
    selectDraft(targetId, { discardBuffer: true })
  }
  viewMode.value = 'edit'
}

async function onSave() {
  const id = selectedId.value
  if (!id || saving.value) return
  if (!isDirty.value) {
    toast.show(t('pages.projectDetail.requirementDrafts.noUnsavedToast'))
    return
  }
  const title = editTitle.value.replace(/^\s+|\s+$/g, '')
  if (!title) {
    titleError.value = t('pages.projectDetail.requirementDrafts.titleRequired')
    return
  }
  titleError.value = ''
  saving.value = true
  try {
    const saved = await api.updateRequirementDraft(props.projectId, id, {
      title,
      bodyMarkdown: editBody.value,
    })
    editTitle.value = saved.title
    editBody.value = saved.bodyMarkdown || ''
    applySavedBaseline(saved.title, saved.bodyMarkdown || '')
    await loadList({ preferId: saved.id, keepSelection: true })
    const cur = items.value.find((d) => d.id === saved.id)
    if (cur) {
      createdAtLabel.value = fmtTime(cur.createdAt)
      updatedAtLabel.value = fmtTime(cur.updatedAt)
      selectedStatus.value = cur.status
    }
    toast.success(t('pages.projectDetail.requirementDrafts.savedToast'))
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function onToggleStatus() {
  const id = selectedId.value
  if (!id || statusBusy.value) return
  const next = selectedStatus.value === 'open' ? 'done' : 'open'
  statusBusy.value = true
  try {
    const patched = await api.patchRequirementDraftStatus(props.projectId, id, next)
    selectedStatus.value = patched.status
    createdAtLabel.value = fmtTime(patched.createdAt)
    updatedAtLabel.value = fmtTime(patched.updatedAt)
    toast.success(
      next === 'done'
        ? t('pages.projectDetail.requirementDrafts.archivedToast')
        : t('pages.projectDetail.requirementDrafts.reopenedToast'),
    )
    await loadList({ keepSelection: true })
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.statusFailed'))
  } finally {
    statusBusy.value = false
  }
}

async function onConfirmDelete() {
  const id = selectedId.value
  if (!id || deleting.value) return
  deleting.value = true
  try {
    await api.deleteRequirementDraft(props.projectId, id)
    showDelete.value = false
    selectedId.value = null
    editTitle.value = ''
    editBody.value = ''
    applySavedBaseline('', '')
    titleError.value = ''
    toast.success(t('pages.projectDetail.requirementDrafts.deletedToast'))
    await loadList()
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.deleteFailed'))
  } finally {
    deleting.value = false
  }
}

function setFilter(f: RequirementDraftStatusFilter) {
  if (filter.value === f) return
  filter.value = f
}

let searchTimer: ReturnType<typeof setTimeout> | null = null
function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    void loadList({ keepSelection: true })
  }, 200)
}

function applyInsert(cmd: DraftInsertCmd) {
  const ta = srcEl.value
  const start = ta?.selectionStart ?? editBody.value.length
  const end = ta?.selectionEnd ?? start
  const next = insertDraftMarkdown({
    value: editBody.value,
    selectionStart: start,
    selectionEnd: end,
    cmd,
  })
  editBody.value = next.value
  void nextTick(() => {
    ta?.focus()
    ta?.setSelectionRange(next.selectionStart, next.selectionEnd)
    syncOverlayScroll()
  })
}

function openFind() {
  findOpen.value = true
  void nextTick(() => findInputEl.value?.focus())
}

function closeFind() {
  findOpen.value = false
  srcEl.value?.focus()
}

function runFindNext() {
  const ta = srcEl.value
  const hit = findNextDraftMatch({
    value: editBody.value,
    query: findQuery.value,
    from: ta?.selectionEnd || 0,
  })
  if (!hit) {
    toast.show(t('pages.projectDetail.requirementDrafts.findNoMatch'))
    return
  }
  ta?.focus()
  ta?.setSelectionRange(hit.index, hit.end)
  if (ta) ta.scrollTop = hit.scrollTop
  syncOverlayScroll()
}

function togglePreviewCollapsed() {
  if (isMobile.value) return
  previewCollapsed.value = !previewCollapsed.value
}

function onSashDown(e: MouseEvent) {
  if (isMobile.value || previewCollapsed.value) return
  e.preventDefault()
  sashDragging.value = true
  const split = splitEl.value
  function move(ev: MouseEvent) {
    if (!split) return
    const r = split.getBoundingClientRect()
    if (r.width <= 0) return
    splitRatio.value = clampDraftSplitRatio((ev.clientX - r.left) / r.width)
  }
  function up() {
    sashDragging.value = false
    document.removeEventListener('mousemove', move)
    document.removeEventListener('mouseup', up)
  }
  document.addEventListener('mousemove', move)
  document.addEventListener('mouseup', up)
}

function syncOverlayScroll() {
  const ta = srcEl.value
  if (!ta) return
  if (hlEl.value) {
    hlEl.value.scrollTop = ta.scrollTop
    hlEl.value.scrollLeft = ta.scrollLeft
  }
  if (gutterEl.value) gutterEl.value.scrollTop = ta.scrollTop
}

function onSrcScroll() {
  syncOverlayScroll()
  if (previewCollapsed.value || !showSplitPreview.value) return
  if (!srcEl.value || !previewEl.value) return
  if (scrollLock) return
  scrollLock = true
  syncScrollTop(srcEl.value, previewEl.value)
  scrollLock = false
}

function onPreviewScroll() {
  if (previewCollapsed.value || !showSplitSource.value) return
  if (!srcEl.value || !previewEl.value) return
  if (scrollLock) return
  scrollLock = true
  syncScrollTop(previewEl.value, srcEl.value)
  syncOverlayScroll()
  scrollLock = false
}

function onDetailKeydown(e: KeyboardEvent) {
  const meta = e.metaKey || e.ctrlKey
  if (!meta) return
  if (e.key.toLowerCase() === 's') {
    e.preventDefault()
    void onSave()
  }
}

function onSrcKeydown(e: KeyboardEvent) {
  const meta = e.metaKey || e.ctrlKey
  if (!meta) return
  const k = e.key.toLowerCase()
  if (k === 'b') {
    e.preventDefault()
    applyInsert('bold')
    return
  }
  if (k === 'i') {
    e.preventDefault()
    applyInsert('italic')
    return
  }
  if (k === 'f') {
    e.preventDefault()
    openFind()
  }
}

function onFindInputKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    runFindNext()
  }
}

function onBeforeUnload(e: BeforeUnloadEvent) {
  if (!isDirty.value) return
  e.preventDefault()
  e.returnValue = ''
}

function kindLabel(kind: RequirementDraftKind) {
  return kind === 'milestone'
    ? t('pages.projectDetail.requirementDrafts.kindMilestone')
    : t('pages.projectDetail.requirementDrafts.kindRequirement')
}

function rowBarStyle(row: GanttRow) {
  const range = draftBarRange(row.draft)
  if (!range) return null
  return barStyle(range.start, range.end, timelineWindow.value)
}

function isRowSelected(id: string) {
  return selectedId.value === id
}

watch(
  () => props.projectId,
  () => {
    filter.value = 'open'
    query.value = ''
    viewMode.value = 'gantt'
    ganttScale.value = 'week'
    selectedId.value = null
    editTitle.value = ''
    editBody.value = ''
    applySavedBaseline('', '')
    void loadList()
  },
)

watch(filter, () => {
  void loadList({ keepSelection: true })
})

watch(editBody, () => {
  schedulePreview()
  scheduleHighlight()
})

watch(isMobile, (narrow) => {
  if (narrow) mobilePane.value = 'src'
})

onMounted(() => {
  schedulePreview()
  scheduleHighlight()
  void loadList()
  window.addEventListener('beforeunload', onBeforeUnload)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  if (previewRaf) cancelAnimationFrame(previewRaf)
  if (highlightRaf) cancelAnimationFrame(highlightRaf)
  if (searchTimer) clearTimeout(searchTimer)
  if (leaveWaiter) {
    leaveWaiter(false)
    leaveWaiter = null
  }
})

defineExpose({
  get isDirty() {
    return isDirty.value
  },
  requestLeave,
})
</script>

<template>
  <div
    class="requirement-drafts flex min-h-[520px] flex-col border border-line bg-base"
    data-testid="requirement-drafts-panel"
  >
    <!-- Top toolbar -->
    <div class="flex flex-wrap items-center gap-2 border-b border-line p-3">
      <div class="seg" data-testid="requirement-drafts-view-segment">
        <button
          type="button"
          :class="{ on: viewMode === 'edit' }"
          data-testid="requirement-drafts-view-edit"
          @click="setViewMode('edit')"
        >
          {{ t('pages.projectDetail.requirementDrafts.viewEdit') }}
        </button>
        <button
          type="button"
          :class="{ on: viewMode === 'gantt' }"
          data-testid="requirement-drafts-view-gantt"
          @click="setViewMode('gantt')"
        >
          {{ t('pages.projectDetail.requirementDrafts.viewGantt') }}
        </button>
        <button
          type="button"
          :class="{ on: viewMode === 'milestones' }"
          data-testid="requirement-drafts-view-milestones"
          @click="setViewMode('milestones')"
        >
          {{ t('pages.projectDetail.requirementDrafts.viewMilestones') }}
        </button>
      </div>

      <div class="seg" data-testid="requirement-drafts-filter">
        <button
          type="button"
          :class="{ on: filter === 'open' }"
          data-testid="requirement-drafts-filter-open"
          @click="setFilter('open')"
        >
          {{ t('pages.projectDetail.requirementDrafts.filterOpen') }}
        </button>
        <button
          type="button"
          :class="{ on: filter === 'done' }"
          data-testid="requirement-drafts-filter-done"
          @click="setFilter('done')"
        >
          {{ t('pages.projectDetail.requirementDrafts.filterDone') }}
        </button>
        <button
          type="button"
          :class="{ on: filter === 'all' }"
          data-testid="requirement-drafts-filter-all"
          @click="setFilter('all')"
        >
          {{ t('pages.projectDetail.requirementDrafts.filterAll') }}
        </button>
      </div>

      <AppButton
        variant="primary"
        size="sm"
        :disabled="creating"
        data-testid="requirement-drafts-new"
        @click="onNewClick"
      >
        {{ t('pages.projectDetail.requirementDrafts.new') }}
      </AppButton>

      <input
        v-model="query"
        type="search"
        class="min-w-[160px] flex-1 border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent focus:shadow-[0_0_0_2px_rgba(123,97,255,0.3)]"
        data-testid="requirement-drafts-search"
        :placeholder="t('pages.projectDetail.requirementDrafts.searchPh')"
        @input="onSearchInput"
      />
    </div>

    <!-- Edit view -->
    <div
      v-if="viewMode === 'edit'"
      class="draft-layout grid min-h-[480px] flex-1 grid-cols-1 md:grid-cols-[280px_1fr]"
    >
      <aside class="flex min-h-[480px] flex-col border-b border-line md:border-b-0 md:border-r">
        <div
          class="min-h-0 flex-1 space-y-1.5 overflow-auto p-2"
          data-testid="requirement-drafts-list"
        >
          <div v-if="loading && !items.length" class="px-5 py-8 text-center text-[13px] text-txt3">
            {{ t('common.loading.label') }}
          </div>
          <div
            v-else-if="!items.length"
            class="px-5 py-8 text-center text-[13px] text-txt3"
            data-testid="requirement-drafts-list-empty"
          >
            {{ t('pages.projectDetail.requirementDrafts.listEmpty') }}
          </div>
          <button
            v-for="d in items"
            :key="d.id"
            type="button"
            class="relative block w-full border border-line px-3 py-2.5 text-left hover:bg-elevated"
            :class="d.id === selectedId ? 'bg-accent-dim' : 'bg-transparent'"
            :data-testid="`requirement-drafts-item-${d.id}`"
            @click="onPickDraft(d.id)"
          >
            <span
              v-if="d.id === selectedId"
              class="absolute bottom-0 left-0 top-0 w-[3px] bg-accent"
              data-testid="requirement-drafts-item-active-bar"
            />
            <div class="mb-1 flex items-center gap-2 text-[13px] font-medium text-txt">
              <span class="min-w-0 truncate">{{ d.title }}</span>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-[11px] text-txt3">
              <span class="border border-line bg-elevated px-1.5 py-px text-txt2">
                {{ kindLabel(d.kind) }}
              </span>
              <span
                class="border px-1.5 py-px"
                :class="
                  d.status === 'open'
                    ? 'border-info/40 text-info bg-elevated'
                    : 'border-ok/40 text-ok bg-elevated'
                "
              >
                {{
                  d.status === 'open'
                    ? t('pages.projectDetail.requirementDrafts.statusOpen')
                    : t('pages.projectDetail.requirementDrafts.statusDone')
                }}
              </span>
              <span>{{ t('pages.projectDetail.requirementDrafts.updated') }} {{ fmtTime(d.updatedAt) }}</span>
            </div>
          </button>
        </div>
      </aside>

      <div
        class="flex min-h-[480px] min-w-0 flex-col"
        data-testid="requirement-drafts-detail"
        @keydown="onDetailKeydown"
      >
        <div
          v-if="!hasSelection"
          class="px-5 py-8 text-center text-[13px] text-txt3"
          data-testid="requirement-drafts-empty-detail"
        >
          <p>{{ t('pages.projectDetail.requirementDrafts.emptyDetail') }}</p>
        </div>
        <template v-else>
          <div class="flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3.5">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <strong class="text-[18px] font-[650] leading-tight text-txt">
                  {{ t('pages.projectDetail.requirementDrafts.editLabel') }}
                </strong>
                <span
                  class="border px-1.5 py-px text-[11px]"
                  :class="
                    selectedStatus === 'open'
                      ? 'border-info/40 text-info bg-elevated'
                      : 'border-ok/40 text-ok bg-elevated'
                  "
                  data-testid="requirement-drafts-status-pill"
                >
                  {{
                    selectedStatus === 'open'
                      ? t('pages.projectDetail.requirementDrafts.statusOpen')
                      : t('pages.projectDetail.requirementDrafts.statusDone')
                  }}
                </span>
                <span
                  v-if="isDirty"
                  class="border border-warn/40 bg-elevated px-1.5 py-px text-[11px] text-warn"
                  data-testid="requirement-drafts-dirty-chip"
                >
                  {{ t('pages.projectDetail.requirementDrafts.unsaved') }}
                </span>
              </div>
              <div class="mt-1 flex flex-wrap gap-3 text-[11px] text-txt3">
                <span>
                  {{ t('pages.projectDetail.requirementDrafts.created') }}
                  <span data-testid="requirement-drafts-created-at">{{ createdAtLabel }}</span>
                </span>
                <span>
                  {{ t('pages.projectDetail.requirementDrafts.updated') }}
                  <span data-testid="requirement-drafts-updated-at">{{ updatedAtLabel }}</span>
                </span>
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <AppButton
                variant="outline"
                size="sm"
                :disabled="statusBusy"
                data-testid="requirement-drafts-toggle-status"
                @click="onToggleStatus"
              >
                {{
                  selectedStatus === 'open'
                    ? t('pages.projectDetail.requirementDrafts.markDone')
                    : t('pages.projectDetail.requirementDrafts.markOpen')
                }}
              </AppButton>
              <AppButton
                variant="outline"
                size="sm"
                class="text-err"
                data-testid="requirement-drafts-delete"
                @click="showDelete = true"
              >
                {{ t('common.buttons.delete') }}
              </AppButton>
              <AppButton
                variant="primary"
                size="sm"
                :disabled="saving"
                data-testid="requirement-drafts-save"
                @click="onSave"
              >
                {{ t('common.buttons.save') }}
              </AppButton>
            </div>
          </div>
          <div class="flex min-h-0 flex-1 flex-col px-4 pb-4 pt-3">
            <div>
              <label class="mb-1.5 block text-xs text-txt2" for="requirement-draft-title">
                {{ t('pages.projectDetail.requirementDrafts.titleLabel') }}
              </label>
              <input
                id="requirement-draft-title"
                v-model="editTitle"
                type="text"
                class="w-full border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent focus:shadow-[0_0_0_2px_rgba(123,97,255,0.3)]"
                data-testid="requirement-drafts-title"
                :placeholder="t('pages.projectDetail.requirementDrafts.titlePh')"
              />
              <div class="mt-1 min-h-[18px] text-xs text-err" data-testid="requirement-drafts-title-error">
                {{ titleError }}
              </div>
            </div>

            <!-- Schedule block (edit view) -->
            <div
              class="rd-schedule mb-4 border border-accent bg-surface p-3"
              data-testid="requirement-drafts-schedule-block"
            >
              <div class="mb-2 text-[13px] font-medium text-txt">
                {{ t('pages.projectDetail.requirementDrafts.scheduleBlockTitle') }}
              </div>
              <p class="mb-3 text-[11px] text-txt3">
                {{ t('pages.projectDetail.requirementDrafts.scheduleHint') }}
              </p>
              <div class="grid gap-3 sm:grid-cols-2">
                <label class="block text-xs text-txt2">
                  {{ t('pages.projectDetail.requirementDrafts.kindLabel') }}
                  <select
                    v-model="editKind"
                    class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                    data-testid="requirement-drafts-schedule-kind"
                    :disabled="scheduleBusy"
                    @change="onScheduleKindChange"
                  >
                    <option value="requirement">
                      {{ t('pages.projectDetail.requirementDrafts.kindRequirement') }}
                    </option>
                    <option value="milestone">
                      {{ t('pages.projectDetail.requirementDrafts.kindMilestone') }}
                    </option>
                  </select>
                </label>
                <label v-if="editKind !== 'milestone'" class="block text-xs text-txt2">
                  {{ t('pages.projectDetail.requirementDrafts.startAtLabel') }}
                  <input
                    v-model="editStartAt"
                    type="date"
                    class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                    data-testid="requirement-drafts-schedule-start"
                    :disabled="scheduleBusy"
                    @change="onScheduleStartChange"
                  />
                </label>
                <label class="block text-xs text-txt2">
                  {{ t('pages.projectDetail.requirementDrafts.dueAtLabel') }}
                  <input
                    v-model="editDueAt"
                    type="date"
                    class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                    data-testid="requirement-drafts-schedule-due"
                    :disabled="scheduleBusy"
                    @change="onScheduleDueChange"
                  />
                </label>
                <label class="block text-xs text-txt2">
                  {{ t('pages.projectDetail.requirementDrafts.progressLabel') }}
                  <input
                    v-model.number="editProgress"
                    type="number"
                    min="0"
                    max="100"
                    class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                    data-testid="requirement-drafts-schedule-progress"
                    :disabled="scheduleBusy"
                    @change="onScheduleProgressChange"
                  />
                </label>
                <label v-if="canEditParent" class="block text-xs text-txt2 sm:col-span-2">
                  {{ t('pages.projectDetail.requirementDrafts.parentLabel') }}
                  <select
                    v-model="editParentId"
                    class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                    data-testid="requirement-drafts-schedule-parent"
                    :disabled="scheduleBusy"
                    @change="onScheduleParentChange"
                  >
                    <option :value="null">
                      {{ t('pages.projectDetail.requirementDrafts.parentNone') }}
                    </option>
                    <option v-for="p in parentOptions" :key="p.id" :value="p.id">
                      {{ p.title }}
                    </option>
                  </select>
                </label>
              </div>
              <div
                v-if="scheduleError"
                class="mt-2 text-xs text-err"
                data-testid="requirement-drafts-schedule-error"
              >
                {{ scheduleError }}
              </div>
            </div>

            <div class="flex min-h-0 flex-1 flex-col">
              <label class="mb-1.5 block text-xs text-txt2">
                {{ t('pages.projectDetail.requirementDrafts.bodyLabel') }}
              </label>
              <div
                class="rd-toolbar flex flex-wrap items-center gap-0.5 border border-b-0 border-line bg-elevated p-1.5"
                data-testid="requirement-drafts-toolbar"
              >
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-h1"
                  :title="t('pages.projectDetail.requirementDrafts.tbH1')"
                  @click="applyInsert('h1')"
                >
                  H1
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-h2"
                  :title="t('pages.projectDetail.requirementDrafts.tbH2')"
                  @click="applyInsert('h2')"
                >
                  H2
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-h3"
                  :title="t('pages.projectDetail.requirementDrafts.tbH3')"
                  @click="applyInsert('h3')"
                >
                  H3
                </button>
                <span class="rd-tb-sep" />
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-bold"
                  :title="t('pages.projectDetail.requirementDrafts.tbBold')"
                  @click="applyInsert('bold')"
                >
                  B
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-italic"
                  :title="t('pages.projectDetail.requirementDrafts.tbItalic')"
                  @click="applyInsert('italic')"
                >
                  I
                </button>
                <span class="rd-tb-sep" />
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-ul"
                  :title="t('pages.projectDetail.requirementDrafts.tbUl')"
                  @click="applyInsert('ul')"
                >
                  · 列
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-ol"
                  :title="t('pages.projectDetail.requirementDrafts.tbOl')"
                  @click="applyInsert('ol')"
                >
                  1. 列
                </button>
                <span class="rd-tb-sep" />
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-link"
                  :title="t('pages.projectDetail.requirementDrafts.tbLink')"
                  @click="applyInsert('link')"
                >
                  链接
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-code"
                  :title="t('pages.projectDetail.requirementDrafts.tbCode')"
                  @click="applyInsert('code')"
                >
                  `
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-fence"
                  :title="t('pages.projectDetail.requirementDrafts.tbFence')"
                  @click="applyInsert('fence')"
                >
                  ```
                </button>
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-table"
                  :title="t('pages.projectDetail.requirementDrafts.tbTable')"
                  @click="applyInsert('table')"
                >
                  表格
                </button>
                <span class="rd-tb-grow" />
                <button
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-find"
                  :title="t('pages.projectDetail.requirementDrafts.tbFind')"
                  @click="openFind"
                >
                  {{ t('pages.projectDetail.requirementDrafts.find') }}
                </button>
                <button
                  v-if="!isMobile"
                  type="button"
                  class="rd-tb"
                  data-testid="requirement-drafts-tb-collapse"
                  :title="t('pages.projectDetail.requirementDrafts.collapsePreview')"
                  @click="togglePreviewCollapsed"
                >
                  {{
                    previewCollapsed
                      ? t('pages.projectDetail.requirementDrafts.expandPreview')
                      : t('pages.projectDetail.requirementDrafts.collapsePreview')
                  }}
                </button>
              </div>
              <div
                v-if="isMobile"
                class="flex gap-1.5 border border-b-0 border-line bg-elevated p-1.5"
                data-testid="requirement-drafts-mobile-switch"
              >
                <button
                  type="button"
                  class="flex-1 border px-2 py-1 text-xs"
                  :class="mobilePane === 'src' ? 'border-accent bg-accent-dim text-txt' : 'border-line text-txt2'"
                  data-testid="requirement-drafts-mobile-src"
                  @click="mobilePane = 'src'"
                >
                  {{ t('pages.projectDetail.requirementDrafts.source') }}
                </button>
                <button
                  type="button"
                  class="flex-1 border px-2 py-1 text-xs"
                  :class="mobilePane === 'prev' ? 'border-accent bg-accent-dim text-txt' : 'border-line text-txt2'"
                  data-testid="requirement-drafts-mobile-prev"
                  @click="mobilePane = 'prev'"
                >
                  {{ t('pages.projectDetail.requirementDrafts.preview') }}
                </button>
              </div>
              <div
                ref="splitEl"
                class="rd-split flex min-h-[360px] flex-1 border border-line bg-surface"
                :class="{
                  'rd-split-narrow': isMobile,
                  'rd-split-collapsed': previewCollapsed && !isMobile,
                }"
                data-testid="requirement-drafts-markdown-split"
              >
                <div
                  v-show="showSplitSource"
                  class="rd-pane flex min-w-0 flex-col"
                  :style="sourceWidthStyle"
                  data-testid="requirement-drafts-source-pane"
                >
                  <div class="flex h-7 items-center justify-between border-b border-line bg-elevated px-2.5 text-[11px] tracking-wide text-txt3">
                    <span>{{ t('pages.projectDetail.requirementDrafts.source') }}</span>
                  </div>
                  <div
                    v-show="findOpen"
                    class="flex items-center gap-1.5 border-b border-line bg-overlay px-2 py-1.5"
                    data-testid="requirement-drafts-findbar"
                  >
                    <input
                      ref="findInputEl"
                      v-model="findQuery"
                      type="search"
                      class="min-w-0 flex-1 border border-line bg-base px-2 py-1 text-[12px] text-txt outline-none focus:border-accent"
                      data-testid="requirement-drafts-find-input"
                      :placeholder="t('pages.projectDetail.requirementDrafts.findPh')"
                      @keydown="onFindInputKeydown"
                    />
                    <button
                      type="button"
                      class="rd-tb"
                      data-testid="requirement-drafts-find-next"
                      @click="runFindNext"
                    >
                      {{ t('pages.projectDetail.requirementDrafts.findNext') }}
                    </button>
                    <button
                      type="button"
                      class="rd-tb"
                      data-testid="requirement-drafts-find-close"
                      @click="closeFind"
                    >
                      {{ t('pages.projectDetail.requirementDrafts.findClose') }}
                    </button>
                  </div>
                  <div class="rd-src-wrap relative flex min-h-0 flex-1">
                    <div
                      ref="gutterEl"
                      class="rd-gutter select-none overflow-hidden border-r border-line bg-elevated py-2.5 text-right font-mono text-[12px] leading-[1.6] text-txt3"
                      aria-hidden="true"
                      data-testid="requirement-drafts-gutter"
                    >
                      <div v-for="n in lineNumbers" :key="n" class="pr-2">{{ n }}</div>
                    </div>
                    <div class="relative min-h-0 min-w-0 flex-1">
                      <pre
                        ref="hlEl"
                        class="rd-hl pointer-events-none absolute inset-0 m-0 overflow-auto bg-base p-2.5 font-mono text-[12px] leading-[1.6] text-txt"
                        aria-hidden="true"
                        data-testid="requirement-drafts-highlight"
                        v-html="highlightHtml"
                      />
                      <textarea
                        ref="srcEl"
                        v-model="editBody"
                        class="rd-src absolute inset-0 z-[1] m-0 resize-none overflow-auto border-none bg-transparent p-2.5 font-mono text-[12px] leading-[1.6] text-transparent caret-txt outline-none"
                        data-testid="requirement-drafts-body"
                        spellcheck="false"
                        :placeholder="t('pages.projectDetail.requirementDrafts.bodyPh')"
                        @scroll="onSrcScroll"
                        @keydown="onSrcKeydown"
                      />
                    </div>
                  </div>
                </div>
                <div
                  v-show="showSplitSource && showSplitPreview"
                  class="rd-sash w-[5px] shrink-0 cursor-col-resize bg-line"
                  :class="sashDragging ? 'bg-accent' : 'hover:bg-accent'"
                  data-testid="requirement-drafts-sash"
                  :title="t('pages.projectDetail.requirementDrafts.sashTitle')"
                  @mousedown="onSashDown"
                />
                <div
                  v-show="showSplitPreview"
                  class="rd-pane flex min-w-0 flex-1 flex-col"
                  data-testid="requirement-drafts-preview-pane"
                >
                  <div class="flex h-7 items-center justify-between border-b border-line bg-elevated px-2.5 text-[11px] tracking-wide text-txt3">
                    <span>{{ t('pages.projectDetail.requirementDrafts.preview') }}</span>
                  </div>
                  <div
                    ref="previewEl"
                    class="md min-h-0 flex-1 overflow-auto bg-base px-3.5 py-3.5 text-[13px] leading-relaxed text-txt"
                    data-testid="requirement-drafts-preview"
                    @scroll="onPreviewScroll"
                    v-html="previewHtml"
                  />
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Gantt view -->
    <div
      v-else-if="viewMode === 'gantt'"
      class="flex min-h-[480px] flex-1 flex-col lg:flex-row"
    >
      <div
        class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
        data-testid="requirement-drafts-gantt"
      >
        <div class="flex items-center gap-2 border-b border-line px-3 py-2">
          <div class="seg" data-testid="requirement-drafts-scale-segment">
            <button
              type="button"
              :class="{ on: ganttScale === 'day' }"
              data-testid="requirement-drafts-scale-day"
              @click="setGanttScale('day')"
            >
              {{ t('pages.projectDetail.requirementDrafts.scaleDay') }}
            </button>
            <button
              type="button"
              :class="{ on: ganttScale === 'week' }"
              data-testid="requirement-drafts-scale-week"
              @click="setGanttScale('week')"
            >
              {{ t('pages.projectDetail.requirementDrafts.scaleWeek') }}
            </button>
            <button
              type="button"
              :class="{ on: ganttScale === 'month' }"
              data-testid="requirement-drafts-scale-month"
              @click="setGanttScale('month')"
            >
              {{ t('pages.projectDetail.requirementDrafts.scaleMonth') }}
            </button>
          </div>
        </div>

        <div v-if="loading && !items.length" class="px-5 py-8 text-center text-[13px] text-txt3">
          {{ t('common.loading.label') }}
        </div>

        <div v-else class="min-h-0 flex-1 overflow-auto">
          <!-- Unscheduled zone -->
          <div
            v-if="ganttRows.unscheduled.length"
            class="border-b border-line bg-elevated"
            data-testid="requirement-drafts-unscheduled"
          >
            <div class="border-b border-line px-3 py-1.5 text-[11px] font-medium uppercase tracking-wide text-txt3">
              {{ t('pages.projectDetail.requirementDrafts.unscheduledTitle') }}
            </div>
            <div
              v-for="row in ganttRows.unscheduled"
              :key="'u-' + row.draft.id"
              class="rd-gantt-row flex cursor-pointer border-b border-line hover:bg-surface"
              :class="isRowSelected(row.draft.id) ? 'bg-accent-dim' : ''"
              @click="onSelectFromGantt(row.draft.id)"
            >
              <div class="rd-gantt-name sticky left-0 z-[2] shrink-0 border-r border-line bg-elevated px-3 py-2 text-[13px] text-txt">
                <span v-if="row.contextual" class="mr-1 text-[10px] text-txt3">
                  {{ t('pages.projectDetail.requirementDrafts.contextualParentHint') }}
                </span>
                {{ row.draft.title }}
              </div>
              <div class="min-w-[200px] flex-1 px-2 py-2 text-[11px] text-txt3">—</div>
            </div>
          </div>

          <!-- Scheduled timeline -->
          <div class="rd-gantt-scroll relative min-w-[640px]">
            <div class="rd-gantt-header flex border-b border-line bg-surface">
              <div class="rd-gantt-name sticky left-0 z-[3] shrink-0 border-r border-line bg-surface px-3 py-2 text-[11px] text-txt3">
                &nbsp;
              </div>
              <div class="relative min-w-0 flex-1">
                <div class="flex">
                  <div
                    v-for="tick in timelineWindow.ticks"
                    :key="tick"
                    class="rd-gantt-tick flex-1 border-r border-line px-1 py-2 text-center text-[10px] text-txt3"
                  >
                    {{ tickLabel(tick, ganttScale) }}
                  </div>
                </div>
                <div
                  class="rd-gantt-today pointer-events-none absolute bottom-0 top-0 z-[1] w-px bg-warn"
                  :style="todayLineStyle"
                  :title="t('pages.projectDetail.requirementDrafts.todayLabel')"
                />
              </div>
            </div>

            <div
              v-if="!ganttRows.scheduled.length && !ganttRows.unscheduled.length"
              class="px-5 py-8 text-center text-[13px] text-txt3"
            >
              {{ t('pages.projectDetail.requirementDrafts.listEmpty') }}
            </div>
            <div
              v-else-if="!ganttRows.scheduled.length"
              class="px-5 py-6 text-center text-[13px] text-txt3"
            >
              {{ t('pages.projectDetail.requirementDrafts.ganttEmptyScheduled') }}
            </div>

            <div
              v-for="row in ganttRows.scheduled"
              :key="'s-' + row.draft.id"
              class="rd-gantt-row relative flex cursor-pointer border-b border-line hover:bg-elevated"
              :class="isRowSelected(row.draft.id) ? 'bg-accent-dim' : ''"
              @click="onSelectFromGantt(row.draft.id)"
            >
              <div
                class="rd-gantt-name sticky left-0 z-[2] shrink-0 border-r border-line bg-base px-3 py-2 text-[13px] text-txt"
                :class="row.indent ? 'pl-6' : ''"
              >
                <span v-if="row.contextual" class="mr-1 text-[10px] text-txt3">
                  {{ t('pages.projectDetail.requirementDrafts.contextualParentHint') }}
                </span>
                <span
                  v-if="row.rowKind === 'group'"
                  class="text-[11px] text-txt3"
                  :title="t('pages.projectDetail.requirementDrafts.groupRowHint')"
                >
                  {{ row.draft.title }}
                </span>
                <span v-else>{{ row.draft.title }}</span>
              </div>
              <div class="relative min-w-0 flex-1 py-2">
                <div
                  class="rd-gantt-today pointer-events-none absolute bottom-0 top-0 z-[1] w-px bg-warn"
                  :style="todayLineStyle"
                />
                <template v-if="row.rowKind === 'bar' && rowBarStyle(row)">
                  <div
                    class="rd-gantt-bar absolute top-1/2 h-[18px] -translate-y-1/2 cursor-pointer"
                    :style="rowBarStyle(row)!"
                    @click.stop="onSelectFromGantt(row.draft.id)"
                  >
                    <div
                      class="rd-gantt-bar-fill h-full"
                      :style="{ width: clampProgress(row.draft.progress) + '%' }"
                    />
                  </div>
                </template>
                <template v-else-if="row.rowKind === 'milestone' && row.draft.dueAt">
                  <div
                    class="rd-gantt-diamond absolute top-1/2 h-3 w-3 -translate-y-1/2 rotate-45 cursor-pointer"
                    :style="diamondStyle(row.draft.dueAt, timelineWindow)"
                    @click.stop="onSelectFromGantt(row.draft.id)"
                  />
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Inspector -->
      <aside
        class="flex w-full shrink-0 flex-col border-t border-line bg-surface lg:w-[300px] lg:border-l lg:border-t-0"
        data-testid="requirement-drafts-inspector"
      >
        <div class="border-b border-line px-3 py-2.5 text-[13px] font-medium text-txt">
          {{ t('pages.projectDetail.requirementDrafts.inspectorTitle') }}
        </div>
        <div v-if="!hasSelection" class="flex-1 px-3 py-6 text-[13px] text-txt3">
          {{ t('pages.projectDetail.requirementDrafts.inspectorEmpty') }}
        </div>
        <div v-else class="flex flex-1 flex-col gap-3 overflow-auto p-3">
          <div class="text-[13px] font-medium text-txt">{{ selectedDraft?.title }}</div>
          <div class="grid gap-3">
            <label class="block text-xs text-txt2">
              {{ t('pages.projectDetail.requirementDrafts.kindLabel') }}
              <select
                v-model="editKind"
                class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                data-testid="requirement-drafts-inspector-kind"
                :disabled="scheduleBusy"
                @change="onScheduleKindChange"
              >
                <option value="requirement">
                  {{ t('pages.projectDetail.requirementDrafts.kindRequirement') }}
                </option>
                <option value="milestone">
                  {{ t('pages.projectDetail.requirementDrafts.kindMilestone') }}
                </option>
              </select>
            </label>
            <label v-if="editKind !== 'milestone'" class="block text-xs text-txt2">
              {{ t('pages.projectDetail.requirementDrafts.startAtLabel') }}
              <input
                v-model="editStartAt"
                type="date"
                class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                data-testid="requirement-drafts-inspector-start"
                :disabled="scheduleBusy"
                @change="onScheduleStartChange"
              />
            </label>
            <label class="block text-xs text-txt2">
              {{ t('pages.projectDetail.requirementDrafts.dueAtLabel') }}
              <input
                v-model="editDueAt"
                type="date"
                class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                data-testid="requirement-drafts-inspector-due"
                :disabled="scheduleBusy"
                @change="onScheduleDueChange"
              />
            </label>
            <label class="block text-xs text-txt2">
              {{ t('pages.projectDetail.requirementDrafts.progressLabel') }}
              <input
                v-model.number="editProgress"
                type="number"
                min="0"
                max="100"
                class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                data-testid="requirement-drafts-inspector-progress"
                :disabled="scheduleBusy"
                @change="onScheduleProgressChange"
              />
            </label>
            <label v-if="canEditParent" class="block text-xs text-txt2">
              {{ t('pages.projectDetail.requirementDrafts.parentLabel') }}
              <select
                v-model="editParentId"
                class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                data-testid="requirement-drafts-inspector-parent"
                :disabled="scheduleBusy"
                @change="onScheduleParentChange"
              >
                <option :value="null">
                  {{ t('pages.projectDetail.requirementDrafts.parentNone') }}
                </option>
                <option v-for="p in parentOptions" :key="p.id" :value="p.id">
                  {{ p.title }}
                </option>
              </select>
            </label>
          </div>
          <div v-if="scheduleError" class="text-xs text-err" data-testid="requirement-drafts-inspector-error">
            {{ scheduleError }}
          </div>
          <AppButton
            variant="outline"
            size="sm"
            data-testid="requirement-drafts-open-body"
            @click="openBody()"
          >
            {{ t('pages.projectDetail.requirementDrafts.openBody') }}
          </AppButton>
        </div>
      </aside>
    </div>

    <!-- Milestones view -->
    <div v-else class="min-h-[480px] flex-1 overflow-auto p-4">
      <div v-if="loading && !items.length" class="px-5 py-8 text-center text-[13px] text-txt3">
        {{ t('common.loading.label') }}
      </div>
      <div
        v-else-if="!milestoneItems.length"
        class="px-5 py-8 text-center text-[13px] text-txt3"
        data-testid="requirement-drafts-milestones-empty"
      >
        {{ t('pages.projectDetail.requirementDrafts.milestonesEmpty') }}
      </div>
      <div v-else class="mx-auto max-w-xl">
        <div
          v-for="(m, idx) in milestoneItems"
          :key="m.id"
          class="rd-milestone-row flex cursor-pointer gap-4 border-l-2 border-line py-3 pl-4"
          :class="isRowSelected(m.id) ? 'border-accent bg-accent-dim' : 'hover:bg-elevated'"
          :data-testid="`requirement-drafts-milestone-${m.id}`"
          @click="onPickDraft(m.id)"
        >
          <div class="flex w-6 shrink-0 flex-col items-center pt-1">
            <div class="h-3 w-3 rotate-45 bg-[#34D399]" />
            <div v-if="idx < milestoneItems.length - 1" class="mt-1 w-px flex-1 bg-line" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="text-[13px] font-medium text-txt">{{ m.title }}</div>
            <div class="mt-1 text-[11px] text-txt3">{{ m.dueAt || '—' }} · {{ m.progress }}%</div>
            <div v-if="isRowSelected(m.id)" class="mt-3 grid gap-2 border border-line bg-surface p-3">
              <label class="block text-xs text-txt2">
                {{ t('pages.projectDetail.requirementDrafts.dueAtLabel') }}
                <input
                  v-model="editDueAt"
                  type="date"
                  class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                  data-testid="requirement-drafts-milestone-due"
                  :disabled="scheduleBusy"
                  @change="onScheduleDueChange"
                  @click.stop
                />
              </label>
              <label class="block text-xs text-txt2">
                {{ t('pages.projectDetail.requirementDrafts.progressLabel') }}
                <input
                  v-model.number="editProgress"
                  type="number"
                  min="0"
                  max="100"
                  class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
                  data-testid="requirement-drafts-milestone-progress"
                  :disabled="scheduleBusy"
                  @change="onScheduleProgressChange"
                  @click.stop
                />
              </label>
              <div v-if="scheduleError" class="text-xs text-err">{{ scheduleError }}</div>
              <AppButton
                variant="outline"
                size="sm"
                data-testid="requirement-drafts-open-body"
                @click.stop="openBody(m.id)"
              >
                {{ t('pages.projectDetail.requirementDrafts.openBody') }}
              </AppButton>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- New draft modal -->
    <AppModal
      :open="showNewModal"
      :title="t('pages.projectDetail.requirementDrafts.newModalTitle')"
      :width="440"
      close-on-esc
      data-testid="requirement-drafts-new-modal"
      @close="closeNewModal"
    >
      <p class="mb-4 text-[13px] text-txt2">
        {{ t('pages.projectDetail.requirementDrafts.newModalHint') }}
      </p>
      <div class="mb-4 flex gap-2">
        <button
          type="button"
          class="flex-1 border px-3 py-2 text-[13px]"
          :class="
            newModalKind === 'requirement'
              ? 'border-accent bg-accent-dim text-txt'
              : 'border-line text-txt2'
          "
          data-testid="requirement-drafts-new-kind-requirement"
          @click="newModalKind = 'requirement'"
        >
          {{ t('pages.projectDetail.requirementDrafts.kindRequirement') }}
        </button>
        <button
          type="button"
          class="flex-1 border px-3 py-2 text-[13px]"
          :class="
            newModalKind === 'milestone'
              ? 'border-accent bg-accent-dim text-txt'
              : 'border-line text-txt2'
          "
          data-testid="requirement-drafts-new-kind-milestone"
          @click="newModalKind = 'milestone'"
        >
          {{ t('pages.projectDetail.requirementDrafts.kindMilestone') }}
        </button>
      </div>
      <label
        v-if="newModalKind === 'milestone'"
        class="mb-2 block text-xs text-txt2"
      >
        {{ t('pages.projectDetail.requirementDrafts.milestoneDueLabel') }}
        <input
          v-model="newModalDueAt"
          type="date"
          class="mt-1 w-full border border-line bg-base px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
          data-testid="requirement-drafts-new-milestone-due"
        />
      </label>
      <div v-if="newModalError" class="text-xs text-err" data-testid="requirement-drafts-new-modal-error">
        {{ newModalError }}
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <AppButton
            variant="outline"
            size="sm"
            data-testid="requirement-drafts-new-cancel"
            @click="closeNewModal"
          >
            {{ t('common.buttons.cancel') }}
          </AppButton>
          <AppButton
            variant="primary"
            size="sm"
            :disabled="creating"
            data-testid="requirement-drafts-new-confirm"
            @click="onConfirmCreate"
          >
            {{ t('pages.projectDetail.requirementDrafts.newModalContinue') }}
          </AppButton>
        </div>
      </template>
    </AppModal>

    <AppModal
      :open="showDelete"
      :title="t('pages.projectDetail.requirementDrafts.deleteTitle')"
      :width="440"
      close-on-esc
      @close="showDelete = false"
    >
      <p class="text-[13px] leading-relaxed text-txt2">
        {{ t('pages.projectDetail.requirementDrafts.deleteBody') }}
      </p>
      <template #footer>
        <div class="flex justify-end gap-2">
          <AppButton variant="outline" size="sm" data-testid="requirement-drafts-delete-cancel" @click="showDelete = false">
            {{ t('common.buttons.cancel') }}
          </AppButton>
          <AppButton
            variant="outline"
            size="sm"
            class="text-err"
            :disabled="deleting"
            data-testid="requirement-drafts-delete-confirm"
            @click="onConfirmDelete"
          >
            {{ t('pages.projectDetail.requirementDrafts.deleteConfirm') }}
          </AppButton>
        </div>
      </template>
    </AppModal>

    <AppModal
      :open="showLeave"
      :title="t('pages.projectDetail.requirementDrafts.leaveTitle')"
      :width="440"
      close-on-esc
      @close="resolveLeave(false)"
    >
      <p class="text-[13px] leading-relaxed text-txt2">
        {{ t('pages.projectDetail.requirementDrafts.leaveBody') }}
      </p>
      <template #footer>
        <div class="flex justify-end gap-2">
          <AppButton
            variant="outline"
            size="sm"
            data-testid="requirement-drafts-leave-cancel"
            @click="resolveLeave(false)"
          >
            {{ t('common.buttons.cancel') }}
          </AppButton>
          <AppButton
            variant="primary"
            size="sm"
            data-testid="requirement-drafts-leave-confirm"
            @click="resolveLeave(true)"
          >
            {{ t('pages.projectDetail.requirementDrafts.leaveConfirm') }}
          </AppButton>
        </div>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
/* 分段控件：对齐 ProjectAuditPanel / page.html Demo（浅底 track + 选中浮起） */
.seg {
  display: inline-flex;
  align-items: stretch;
  gap: 2px;
  padding: 3px;
  background: rgb(var(--c-elevated));
  border: 1px solid rgb(var(--c-line));
}
.seg button {
  border: 0;
  background: transparent;
  height: 28px;
  padding: 0 12px;
  font: inherit;
  font-size: 12px;
  line-height: 1.3;
  color: rgb(var(--c-txt2));
  cursor: pointer;
  font-weight: 500;
  white-space: nowrap;
}
.seg button:hover {
  color: rgb(var(--c-txt));
}
.seg button.on {
  background: rgb(var(--c-surface));
  color: rgb(var(--c-txt));
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}
.rd-tb {
  min-width: 28px;
  border: 1px solid transparent;
  border-radius: 0;
  background: transparent;
  padding: 4px 7px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  color: rgb(var(--c-txt2));
}
.rd-tb:hover {
  border-color: rgb(var(--c-line));
  background: rgb(var(--c-overlay));
  color: rgb(var(--c-txt));
}
.rd-tb-sep {
  width: 1px;
  height: 16px;
  margin: 0 4px;
  background: rgb(var(--c-line-strong));
}
.rd-tb-grow {
  flex: 1;
}
.rd-src-wrap {
  min-height: 280px;
}
.rd-gutter {
  width: 36px;
  flex-shrink: 0;
}
.rd-hl,
.rd-src {
  white-space: pre;
  tab-size: 2;
}
.rd-src::placeholder {
  color: rgb(var(--c-txt3));
}
.rd-schedule {
  border-radius: 0;
}
.rd-gantt-name {
  width: 220px;
  min-width: 220px;
}
.rd-gantt-bar {
  border-radius: 0;
  background: #7b61ff;
  min-width: 4px;
}
.rd-gantt-bar-fill {
  border-radius: 0;
  background: rgba(255, 255, 255, 0.35);
  pointer-events: none;
}
.rd-gantt-diamond {
  border-radius: 0;
  background: #34d399;
}
.rd-gantt-today {
  background: #fbbf24;
}
:deep(.rd-tok-head) {
  color: rgb(var(--c-accent-2));
}
:deep(.rd-tok-bold) {
  color: rgb(var(--c-txt));
  font-weight: 700;
}
:deep(.rd-tok-em) {
  color: rgb(var(--c-accent-2));
}
:deep(.rd-tok-code) {
  color: rgb(var(--c-ok));
}
:deep(.rd-tok-link) {
  color: rgb(var(--c-info));
}
</style>

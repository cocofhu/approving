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
import { fmtTime } from '@/lib/shared/format'
import { renderMarkdown } from '@/lib/shared/markdown'
import type { RequirementDraft, RequirementDraftStatusFilter } from '@/lib/shared/types'

const props = defineProps<{
  projectId: string
}>()

const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()

const filter = ref<RequirementDraftStatusFilter>('open')
const query = ref('')
const items = ref<RequirementDraft[]>([])
const loading = ref(false)
const selectedId = ref<string | null>(null)
const creating = ref(false)
const saving = ref(false)
const statusBusy = ref(false)
const deleting = ref(false)
const showDelete = ref(false)
const showLeave = ref(false)

const editTitle = ref('')
const editBody = ref('')
const savedTitle = ref('')
const savedBody = ref('')
const titleError = ref('')
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

const selected = computed(() => items.value.find((d) => d.id === selectedId.value) || null)
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

async function loadList(opts?: { preferId?: string | null; keepSelection?: boolean }) {
  const seq = ++loadSeq
  loading.value = true
  try {
    const res = await api.listRequirementDrafts(props.projectId, {
      status: filter.value,
      q: query.value.trim() || undefined,
    })
    if (seq !== loadSeq) return
    items.value = Array.isArray(res.items) ? res.items : []
    const prefer = opts?.preferId
    if (prefer && items.value.some((d) => d.id === prefer)) {
      selectDraft(prefer, { discardBuffer: opts?.keepSelection ? false : true })
      return
    }
    if (opts?.keepSelection && selectedId.value) {
      const cur = items.value.find((d) => d.id === selectedId.value)
      if (cur) {
        createdAtLabel.value = fmtTime(cur.createdAt)
        updatedAtLabel.value = fmtTime(cur.updatedAt)
        selectedStatus.value = cur.status
      }
      return
    }
    if (selectedId.value && items.value.some((d) => d.id === selectedId.value)) {
      selectDraft(selectedId.value, { discardBuffer: true })
      return
    }
    selectedId.value = null
  } catch (e: any) {
    if (seq !== loadSeq) return
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.loadFailed'))
    items.value = []
    selectedId.value = null
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

function selectDraft(id: string, opts?: { discardBuffer?: boolean }) {
  const d = items.value.find((x) => x.id === id)
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

async function onCreate() {
  if (creating.value) return
  if (isDirty.value) {
    const ok = await requestLeave()
    if (!ok) return
  }
  creating.value = true
  try {
    const created = await api.createRequirementDraft(props.projectId)
    filter.value = 'open'
    query.value = ''
    await loadList({ preferId: created.id })
    toast.success(t('pages.projectDetail.requirementDrafts.createdToast'))
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.requirementDrafts.createFailed'))
  } finally {
    creating.value = false
  }
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
    await api.patchRequirementDraftStatus(props.projectId, id, next)
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

watch(
  () => props.projectId,
  () => {
    filter.value = 'open'
    query.value = ''
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
  if (narrow) {
    mobilePane.value = 'src'
  }
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
    <div class="draft-layout grid min-h-[520px] flex-1 grid-cols-1 md:grid-cols-[280px_1fr]">
      <aside class="flex min-h-[520px] flex-col border-b border-line md:border-b-0 md:border-r">
        <div class="flex flex-col gap-2 border-b border-line p-3">
          <div class="flex gap-2">
            <div class="seg flex flex-1 border border-line" data-testid="requirement-drafts-filter">
              <button
                type="button"
                class="flex-1 bg-transparent px-2 py-1.5 text-xs text-txt2"
                :class="filter === 'open' ? 'bg-accent-dim text-txt' : ''"
                data-testid="requirement-drafts-filter-open"
                @click="setFilter('open')"
              >
                {{ t('pages.projectDetail.requirementDrafts.filterOpen') }}
              </button>
              <button
                type="button"
                class="flex-1 border-l border-line bg-transparent px-2 py-1.5 text-xs text-txt2"
                :class="filter === 'done' ? 'bg-accent-dim text-txt' : ''"
                data-testid="requirement-drafts-filter-done"
                @click="setFilter('done')"
              >
                {{ t('pages.projectDetail.requirementDrafts.filterDone') }}
              </button>
              <button
                type="button"
                class="flex-1 border-l border-line bg-transparent px-2 py-1.5 text-xs text-txt2"
                :class="filter === 'all' ? 'bg-accent-dim text-txt' : ''"
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
              @click="onCreate"
            >
              {{ t('pages.projectDetail.requirementDrafts.new') }}
            </AppButton>
          </div>
          <input
            v-model="query"
            type="search"
            class="w-full border border-line bg-base px-3 py-2 text-[13px] text-txt outline-none focus:border-accent focus:shadow-[0_0_0_2px_rgba(123,97,255,0.3)]"
            data-testid="requirement-drafts-search"
            :placeholder="t('pages.projectDetail.requirementDrafts.searchPh')"
            @input="onSearchInput"
          />
        </div>
        <div class="min-h-0 flex-1 space-y-1.5 overflow-auto p-2" data-testid="requirement-drafts-list">
          <div
            v-if="loading && !items.length"
            class="px-5 py-8 text-center text-[13px] text-txt3"
          >
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
        class="flex min-h-[520px] min-w-0 flex-col"
        data-testid="requirement-drafts-detail"
        @keydown="onDetailKeydown"
      >
        <div
          v-if="!selected"
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
.rd-tb {
  min-width: 28px;
  border: 1px solid transparent;
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

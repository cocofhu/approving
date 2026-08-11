<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api } from '@/lib/api'
import { fmtTime } from '@/lib/format'
import { renderMarkdown } from '@/lib/markdown'
import { useToast } from '@/lib/useToast'
import type { RequirementDraft, RequirementDraftStatusFilter } from '@/lib/types'

const props = defineProps<{
  projectId: string
}>()

const { t } = useI18n()
const toast = useToast()

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

const editTitle = ref('')
const editBody = ref('')
const titleError = ref('')
const createdAtLabel = ref('—')
const updatedAtLabel = ref('—')
const selectedStatus = ref<'open' | 'done'>('open')

const selected = computed(() => items.value.find((d) => d.id === selectedId.value) || null)

const previewHtml = computed(() => {
  const src = editBody.value
  if (!String(src || '').trim()) {
    return `<p class="text-txt3">${t('pages.projectDetail.requirementDrafts.emptyBodyPreview')}</p>`
  }
  return renderMarkdown(src)
})

let loadSeq = 0

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
      selectDraft(prefer, { discardBuffer: true })
      return
    }
    if (opts?.keepSelection && selectedId.value && items.value.some((d) => d.id === selectedId.value)) {
      // keep buffer (e.g. after save)
      const cur = items.value.find((d) => d.id === selectedId.value)!
      createdAtLabel.value = fmtTime(cur.createdAt)
      updatedAtLabel.value = fmtTime(cur.updatedAt)
      selectedStatus.value = cur.status
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
  }
  createdAtLabel.value = fmtTime(d.createdAt)
  updatedAtLabel.value = fmtTime(d.updatedAt)
  selectedStatus.value = d.status
}

async function onCreate() {
  if (creating.value) return
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
    await loadList({ preferId: saved.id, keepSelection: true })
    // After reload, refresh labels from list and keep buffer in sync
    const cur = items.value.find((d) => d.id === saved.id)
    if (cur) {
      selectDraft(cur.id, { discardBuffer: true })
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
    await loadList()
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
    void loadList()
  }, 200)
}

watch(
  () => props.projectId,
  () => {
    filter.value = 'open'
    query.value = ''
    selectedId.value = null
    void loadList()
  },
)

watch(filter, () => {
  void loadList()
})

onMounted(() => {
  void loadList()
})
</script>

<template>
  <div
    class="requirement-drafts flex min-h-[520px] flex-col border border-line bg-base"
    data-testid="requirement-drafts-panel"
  >
    <div class="draft-layout grid min-h-[520px] flex-1 grid-cols-1 md:grid-cols-[300px_1fr]">
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
        <div class="min-h-0 flex-1 overflow-auto" data-testid="requirement-drafts-list">
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
            class="block w-full border-b border-line px-3.5 py-3 text-left hover:bg-elevated"
            :class="d.id === selectedId ? 'bg-accent-dim' : 'bg-transparent'"
            :data-testid="`requirement-drafts-item-${d.id}`"
            @click="selectDraft(d.id)"
          >
            <div class="mb-1 flex items-center gap-2 text-[13px] text-txt">
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

      <div class="flex min-h-[520px] min-w-0 flex-col" data-testid="requirement-drafts-detail">
        <div
          v-if="!selected"
          class="px-5 py-8 text-center text-[13px] text-txt3"
          data-testid="requirement-drafts-empty-detail"
        >
          <p>{{ t('pages.projectDetail.requirementDrafts.emptyDetail') }}</p>
        </div>
        <template v-else>
          <div class="flex flex-wrap items-start justify-between gap-2 border-b border-line px-3.5 py-3">
            <div>
              <div class="flex flex-wrap items-center gap-2">
                <strong class="text-sm text-txt">{{ t('pages.projectDetail.requirementDrafts.editLabel') }}</strong>
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
              </div>
              <div class="mt-1.5 flex flex-wrap gap-3 text-xs text-txt3">
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
          <div class="flex flex-1 flex-col gap-3 p-3.5">
            <div>
              <label class="mb-1.5 block text-xs text-txt3" for="requirement-draft-title">
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
              <div class="mt-1.5 min-h-[18px] text-xs text-err" data-testid="requirement-drafts-title-error">
                {{ titleError }}
              </div>
            </div>
            <div class="flex min-h-0 flex-1 flex-col">
              <label class="mb-1.5 block text-xs text-txt3">
                {{ t('pages.projectDetail.requirementDrafts.bodyLabel') }}
              </label>
              <div
                class="split grid min-h-[280px] flex-1 grid-cols-1 border border-line md:grid-cols-2"
                data-testid="requirement-drafts-markdown-split"
              >
                <div class="flex min-w-0 flex-col border-b border-line md:border-b-0 md:border-r">
                  <div class="border-b border-line bg-elevated px-2.5 py-1.5 text-[11px] text-txt3">
                    {{ t('pages.projectDetail.requirementDrafts.source') }}
                  </div>
                  <textarea
                    v-model="editBody"
                    class="min-h-[240px] flex-1 resize-y border-none bg-base px-3 py-2.5 font-mono text-[13px] leading-relaxed text-txt outline-none"
                    data-testid="requirement-drafts-body"
                    :placeholder="t('pages.projectDetail.requirementDrafts.bodyPh')"
                  />
                </div>
                <div class="flex min-w-0 flex-col">
                  <div class="border-b border-line bg-elevated px-2.5 py-1.5 text-[11px] text-txt3">
                    {{ t('pages.projectDetail.requirementDrafts.preview') }}
                  </div>
                  <div
                    class="md min-h-[240px] flex-1 overflow-auto bg-base px-3 py-2.5 text-[13px] leading-relaxed text-txt"
                    data-testid="requirement-drafts-preview"
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
  </div>
</template>

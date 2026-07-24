<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import StatusPill from '@/components/ui/StatusPill.vue'
import AppModal from '@/components/ui/AppModal.vue'
import RunLaunchModal, { type InputField } from '@/components/workflow/RunLaunchModal.vue'
import CopyWorkflowModal from '@/components/workflow/CopyWorkflowModal.vue'
import ExportVersionModal from '@/components/workflow/ExportVersionModal.vue'
import { api } from '@/lib/api'
import { useWorkflowImport } from '@/lib/useWorkflowImport'
import { fmtTime } from '@/lib/format'
import { clearRunDraft, mergeRunDraft, saveRunDraft } from '@/lib/runDraft'
import { useToast } from '@/lib/useToast'
import { useBreakpoint } from '@/lib/useBreakpoint'
import type { ClarifyImage, Workflow } from '@/lib/types'

const SKELETON_ROWS = 6

/** Persists across route remounts within the same session. */
let hasInitialLoaded = false

const router = useRouter()
const { t } = useI18n()
const toast = useToast()
const { isMobile } = useBreakpoint()
const workflows = ref<Workflow[]>([])
const loading = ref(false)
const initialLoading = ref(false)
const showTableLoading = computed(() => loading.value && hasInitialLoaded)
let requestSeq = 0
let activeLoadingSeq = 0
const runTarget = ref<Workflow | null>(null)
const runFields = ref<InputField[]>([])
const runInputs = ref<Record<string, string>>({})
const runImages = ref<Record<string, ClarifyImage[]>>({})
const draftRestored = ref(false)
const openMenuId = ref<string | null>(null)

// Run-launch inputs are the global variables marked ask=true. Map the unified
// Variable shape onto the dialog's field shape (string→text, paragraph→textarea).
function askFields(w: Workflow): InputField[] {
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

const deleteTarget = ref<Workflow | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const copyPreviewLoading = ref<string | null>(null)
const copyModal = ref<{ sourceId: string; sourceName: string; suggestedName: string } | null>(null)
const exportTarget = ref<Workflow | null>(null)

const { fileInput, triggerImport, handleFileChange } = useWorkflowImport()

const existingNames = () => workflows.value.map((w) => w.name)

function closeMenu() {
  openMenuId.value = null
}

function toggleMenu(id: string) {
  openMenuId.value = openMenuId.value === id ? null : id
}

function openEdit(w: Workflow) {
  closeMenu()
  router.push('/workflows/' + w.id + '/edit')
}

function onCardRun(w: Workflow) {
  closeMenu()
  openRun(w)
}

function onCardCopy(w: Workflow) {
  closeMenu()
  void openCopy(w)
}

function onCardExport(w: Workflow) {
  closeMenu()
  exportTarget.value = w
}

function onCardDelete(w: Workflow) {
  closeMenu()
  openDelete(w)
}

function onDocClick(e: MouseEvent) {
  const el = e.target as Element | null
  if (!el?.closest?.('[data-wf-menu]')) closeMenu()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') closeMenu()
}

function onScrollClose() {
  if (openMenuId.value) closeMenu()
}

function menuIdFor(id: string) {
  return 'wf-more-menu-' + id
}

async function openCopy(w: Workflow) {
  copyPreviewLoading.value = w.id
  try {
    const preview = await api.copyPreviewWorkflow(w.id)
    copyModal.value = {
      sourceId: preview.sourceId,
      sourceName: preview.sourceName,
      suggestedName: preview.suggestedName,
    }
  } catch {
    toast.error(t('common.toast.copyNameFailed'))
  } finally {
    copyPreviewLoading.value = null
  }
}

function closeCopyModal() {
  copyModal.value = null
}

function onCopied(wf: Workflow) {
  workflows.value = [wf, ...workflows.value.filter((x) => x.id !== wf.id)]
  copyModal.value = null
  toast.success(t('common.toast.copiedRedirecting', { name: wf.name }))
  router.push('/workflows/' + wf.id + '/edit')
}

async function load({ showLoading = false }: { showLoading?: boolean } = {}) {
  const localSeq = ++requestSeq
  const isFirstLoad = !hasInitialLoaded

  if (isFirstLoad) {
    initialLoading.value = true
  } else if (showLoading) {
    activeLoadingSeq = localSeq
    loading.value = true
  }

  try {
    const list = await api.listWorkflows()
    if (localSeq === requestSeq) {
      workflows.value = list
    }
  } catch {
    if (localSeq === requestSeq) {
      workflows.value = []
    }
  } finally {
    if (isFirstLoad) {
      hasInitialLoaded = true
      initialLoading.value = false
    } else if (showLoading && activeLoadingSeq === localSeq) {
      loading.value = false
    }
  }
}

onMounted(() => {
  load({ showLoading: true })
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('scroll', onScrollClose, true)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('scroll', onScrollClose, true)
})

function fieldOptions(f: InputField): string[] {
  return String(f.options || '').split(/[,，]/).map((s) => s.trim()).filter(Boolean)
}

function openRun(w: Workflow) {
  runTarget.value = w
  draftRestored.value = false
  runFields.value = askFields(w)
  const seed: Record<string, string> = {}
  const imgSeed: Record<string, ClarifyImage[]> = {}
  for (const f of runFields.value) {
    seed[f.key] = f.default || (f.type === 'select' ? fieldOptions(f)[0] || '' : '')
    imgSeed[f.key] = []
  }
  const keys = runFields.value.map((f) => f.key)
  const merged = mergeRunDraft(w.id, seed, imgSeed, keys)
  runInputs.value = merged.inputs
  runImages.value = merged.images
  draftRestored.value = merged.restored
}

function saveRunDraftClick() {
  const target = runTarget.value
  if (!target) return
  const images: Record<string, ClarifyImage[]> = {}
  for (const [k, v] of Object.entries(runImages.value)) {
    images[k] = v ? [...v] : []
  }
  const result = saveRunDraft(target.id, { ...runInputs.value }, images)
  if (result === 'ok') toast.success(t('common.toast.draftSaved'))
  else if (result === 'quota_exceeded') toast.error(t('common.toast.draftTooLarge'))
  else toast.error(t('common.toast.draftSaveFailed'))
}

function openDelete(w: Workflow) {
  deleteTarget.value = w
  deleteError.value = ''
}
async function confirmDelete() {
  const target = deleteTarget.value
  if (!target) return
  deleting.value = true
  deleteError.value = ''
  try {
    await api.deleteWorkflow(target.id)
    workflows.value = workflows.value.filter((w) => w.id !== target.id)
    deleteTarget.value = null
  } catch (e: any) {
    deleteError.value = String(e?.message || e)
  } finally {
    deleting.value = false
  }
}
function closeRunModal() {
  runTarget.value = null
}

function onRunStayed() {
  void load({ showLoading: true })
}

function onRunStarted() {
  if (runTarget.value) clearRunDraft(runTarget.value.id)
}

function onViewRun(runId: string) {
  router.push('/runs/' + runId)
}
</script>

<template>
  <div>
    <div class="mb-5 flex flex-col gap-2.5 md:flex-row md:items-start md:justify-between">
      <div class="min-w-0">
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.workflowList.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.workflowList.subtitle') }}</p>
      </div>
      <div class="flex w-full gap-2 md:w-auto">
        <AppButton
          class="min-h-[44px] flex-1 md:min-h-0 md:flex-none"
          variant="outline"
          icon="input"
          @click="triggerImport"
        >{{ t('common.buttons.import') }}</AppButton>
        <AppButton
          class="min-h-[44px] flex-1 md:min-h-0 md:flex-none"
          variant="primary"
          icon="plus"
          @click="router.push('/workflows/new/edit')"
        >{{ t('common.buttons.newWorkflow') }}</AppButton>
      </div>
    </div>

    <!-- Mobile card list -->
    <div v-if="isMobile" :class="{ 'table-loading': showTableLoading }">
      <template v-if="initialLoading">
        <div class="flex flex-col gap-2">
          <div
            v-for="n in SKELETON_ROWS"
            :key="'skel-card-' + n"
            class="rounded-lg border border-line bg-surface p-3"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="flex min-w-0 flex-1 items-start gap-2.5">
                <div class="h-8 w-8 shrink-0 rounded-md bg-elevated animate-pulse" />
                <div class="min-w-0 flex-1">
                  <div class="h-3.5 w-[70%] rounded bg-elevated animate-pulse" />
                  <div class="mt-1 h-2.5 w-[50%] rounded bg-elevated animate-pulse" />
                </div>
              </div>
              <div class="h-5 w-14 shrink-0 rounded bg-elevated animate-pulse" />
            </div>
            <div class="mt-3 flex gap-2">
              <div class="h-11 flex-1 rounded-md bg-elevated animate-pulse" />
              <div class="h-11 w-11 shrink-0 rounded-md bg-elevated animate-pulse" />
            </div>
          </div>
        </div>
      </template>
      <div v-else-if="!workflows.length" class="card px-5 py-10 text-center text-[13px] text-txt3">
        {{ t('common.empty.noWorkflows') }}
      </div>
      <div v-else class="flex flex-col gap-2">
        <article
          v-for="w in workflows"
          :key="w.id"
          class="flex flex-col gap-3 rounded-lg border border-line bg-surface p-3 transition hover:border-line-strong hover:bg-elevated"
        >
          <button
            type="button"
            class="flex w-full items-start justify-between gap-3 text-left"
            @click="openEdit(w)"
          >
            <div class="flex min-w-0 flex-1 items-start gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-accent-dim text-accent-2">
                <Icon name="workflow" :size="16" />
              </div>
              <div class="min-w-0">
                <div class="truncate text-sm font-semibold text-txt">{{ w.name }}</div>
                <div
                  v-if="w.description"
                  class="mt-0.5 truncate text-[11px] text-txt3"
                >{{ w.description }}</div>
              </div>
            </div>
            <StatusPill :status="w.status" size="sm" />
          </button>

          <div class="relative flex items-center gap-2" data-wf-menu @click.stop>
            <button
              type="button"
              class="inline-flex min-h-[44px] flex-1 items-center justify-center gap-1.5 rounded-md bg-accent-dim px-3.5 text-sm font-medium text-accent-2 transition hover:brightness-110"
              @click="onCardRun(w)"
            >
              <Icon name="play" :size="14" />
              {{ t('common.buttons.run') }}
            </button>
            <button
              type="button"
              class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-md border border-line bg-surface text-txt2 transition hover:border-line-strong hover:bg-elevated hover:text-txt"
              :class="{ 'border-line-strong bg-elevated text-txt': openMenuId === w.id }"
              :aria-label="t('pages.workflowList.moreActions')"
              :aria-expanded="openMenuId === w.id"
              aria-haspopup="menu"
              :aria-controls="openMenuId === w.id ? menuIdFor(w.id) : undefined"
              @click="toggleMenu(w.id)"
            >
              <Icon name="more" :size="18" />
            </button>
            <div
              v-if="openMenuId === w.id"
              :id="menuIdFor(w.id)"
              class="absolute bottom-[calc(100%+6px)] right-0 z-30 min-w-[148px] rounded-md border border-line-strong bg-surface p-1 shadow-lg"
              role="menu"
            >
              <button
                type="button"
                role="menuitem"
                class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-txt2 transition hover:bg-elevated hover:text-txt"
                @click="openEdit(w)"
              >
                <Icon name="edit" :size="14" />{{ t('common.buttons.edit') }}
              </button>
              <button
                type="button"
                role="menuitem"
                class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-txt2 transition hover:bg-elevated hover:text-txt disabled:opacity-50"
                :disabled="copyPreviewLoading === w.id"
                @click="onCardCopy(w)"
              >
                <Icon name="copy" :size="14" />{{ copyPreviewLoading === w.id ? t('common.buttons.loading') : t('common.buttons.copy') }}
              </button>
              <button
                type="button"
                role="menuitem"
                class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-txt2 transition hover:bg-elevated hover:text-txt"
                @click="onCardExport(w)"
              >
                <Icon name="download" :size="14" />{{ t('common.buttons.export') }}
              </button>
              <button
                type="button"
                role="menuitem"
                class="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 text-left text-[13px] text-err transition hover:bg-err/10"
                @click="onCardDelete(w)"
              >
                <Icon name="trash" :size="14" />{{ t('common.buttons.delete') }}
              </button>
            </div>
          </div>
        </article>
      </div>
    </div>

    <!-- Desktop table -->
    <div v-else class="card overflow-hidden" :class="{ 'table-loading': showTableLoading }">
      <div class="scroll-area overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-[11px] uppercase tracking-wider text-txt3">
              <th class="whitespace-nowrap px-5 py-2.5 font-medium">{{ t('common.table.name') }}</th>
              <th class="whitespace-nowrap px-5 py-2.5 font-medium">{{ t('common.table.status') }}</th>
              <th class="whitespace-nowrap px-5 py-2.5 font-medium">{{ t('common.table.version') }}</th>
              <th class="whitespace-nowrap px-5 py-2.5 font-medium">{{ t('common.table.repo') }}</th>
              <th class="whitespace-nowrap px-5 py-2.5 font-medium">{{ t('common.table.lastRun') }}</th>
              <th class="whitespace-nowrap px-5 py-2.5 font-medium">{{ t('common.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-if="initialLoading">
              <tr v-for="n in SKELETON_ROWS" :key="'skel-' + n" class="border-t border-line">
                <td class="px-5 py-3">
                  <div class="flex items-center gap-2.5">
                    <div class="h-8 w-8 shrink-0 rounded-md bg-elevated animate-pulse" />
                    <div class="min-w-0 flex-1">
                      <div class="h-3.5 w-[70%] rounded bg-elevated animate-pulse" />
                      <div class="mt-1 h-2.5 w-[50%] rounded bg-elevated animate-pulse" />
                    </div>
                  </div>
                </td>
                <td class="px-5 py-3">
                  <div class="h-3.5 w-14 rounded bg-elevated animate-pulse" />
                </td>
                <td class="px-5 py-3">
                  <div class="h-3 w-[40%] rounded bg-elevated animate-pulse" />
                </td>
                <td class="px-5 py-3">
                  <div class="h-3 w-[55%] rounded bg-elevated animate-pulse" />
                </td>
                <td class="px-5 py-3">
                  <div class="h-3 w-[72px] rounded bg-elevated animate-pulse" />
                </td>
                <td class="px-5 py-3">
                  <div class="flex items-center gap-1.5">
                    <div class="h-6 w-12 rounded bg-elevated animate-pulse" />
                    <div class="h-6 w-12 rounded bg-elevated animate-pulse" />
                    <div class="h-6 w-12 rounded bg-elevated animate-pulse" />
                  </div>
                </td>
              </tr>
            </template>
            <tr v-else-if="!workflows.length">
              <td colspan="6" class="px-5 py-10 text-center text-[13px] text-txt3">
                {{ t('common.empty.noWorkflows') }}
              </td>
            </tr>
            <template v-else>
              <tr v-for="w in workflows" :key="w.id" class="border-t border-line transition hover:bg-elevated">
              <td class="px-5 py-3">
                <div class="flex items-center gap-2.5">
                  <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-accent-dim text-accent-2"><Icon name="workflow" :size="16" /></div>
                  <div class="min-w-0">
                    <div class="font-medium text-txt">{{ w.name }}</div>
                    <div class="max-w-[420px] truncate text-[11px] text-txt3">{{ w.description }}</div>
                  </div>
                </div>
              </td>
              <td class="px-5 py-3"><StatusPill :status="w.status" size="sm" /></td>
              <td class="whitespace-nowrap px-5 py-3 text-txt2">v{{ w.version }}</td>
              <td class="px-5 py-3">
                <span class="chip" :class="w.needsRepo ? '' : 'border-ok/30 text-ok'">{{ w.needsRepo ? t('pages.workflowList.repoNeeds') : t('pages.workflowList.repoNotNeeded') }}</span>
              </td>
              <td class="whitespace-nowrap px-5 py-3 text-txt3">{{ fmtTime(w.lastRunAt || '') }}</td>
              <td class="px-5 py-3">
                <div class="flex items-center gap-1.5">
                  <button class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-txt2 hover:bg-overlay hover:text-txt" @click="router.push('/workflows/' + w.id + '/edit')"><Icon name="edit" :size="13" class="mr-1 inline" />{{ t('common.buttons.edit') }}</button>
                  <button class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-accent-2 hover:bg-accent-dim" @click="openRun(w)"><Icon name="play" :size="13" class="mr-1 inline" />{{ t('common.buttons.run') }}</button>
                  <button
                    class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-accent-2 hover:bg-accent-dim disabled:opacity-50"
                    :disabled="copyPreviewLoading === w.id"
                    @click="openCopy(w)"
                  ><Icon name="copy" :size="13" class="mr-1 inline" />{{ copyPreviewLoading === w.id ? t('common.buttons.loading') : t('common.buttons.copy') }}</button>
                  <button
                    class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-accent-2 hover:bg-accent-dim"
                    @click="exportTarget = w"
                  ><Icon name="download" :size="13" class="mr-1 inline" />{{ t('common.buttons.export') }}</button>
                  <button class="whitespace-nowrap rounded-md px-2 py-1 text-xs text-txt2 hover:bg-err/10 hover:text-err" @click="openDelete(w)"><Icon name="trash" :size="13" class="mr-1 inline" />{{ t('common.buttons.delete') }}</button>
                </div>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <RunLaunchModal
      v-if="runTarget"
      :open="!!runTarget"
      :workflow-id="runTarget.id"
      :workflow-name="runTarget.name"
      :fields="runFields"
      :run-inputs="runInputs"
      :run-images="runImages"
      :draft-restored="draftRestored"
      @close="closeRunModal"
      @stayed="onRunStayed"
      @view-run="onViewRun"
      @save-draft="saveRunDraftClick"
      @started="onRunStarted"
    />

    <CopyWorkflowModal
      v-if="copyModal"
      :open="!!copyModal"
      :source-id="copyModal.sourceId"
      :source-name="copyModal.sourceName"
      :suggested-name="copyModal.suggestedName"
      :existing-names="existingNames()"
      @close="closeCopyModal"
      @copied="onCopied"
    />

    <ExportVersionModal
      v-if="exportTarget"
      :open="!!exportTarget"
      :workflow-id="exportTarget.id"
      :workflow-name="exportTarget.name"
      :description="exportTarget.description"
      :needs-repo="exportTarget.needsRepo"
      :status="exportTarget.status"
      @close="exportTarget = null"
    />

    <input ref="fileInput" type="file" accept=".json" class="hidden" @change="handleFileChange" />

    <AppModal :open="!!deleteTarget" :title="t('pages.workflowList.deleteTitle', { name: deleteTarget?.name || '' })" @close="deleteTarget = null">
      <div class="space-y-3 text-sm text-txt2">
        <div class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
          <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
          {{ t('pages.workflowList.deleteWarning') }}
        </div>
        <p>{{ t('pages.workflowList.deleteConfirm', { name: deleteTarget?.name }) }}</p>
        <div v-if="deleteError" class="flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err">
          <Icon name="alert" :size="14" class="mt-0.5" />{{ deleteError }}
        </div>
      </div>
      <template #footer>
        <AppButton variant="ghost" @click="deleteTarget = null">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton variant="danger" icon="trash" :disabled="deleting" @click="confirmDelete">{{ deleting ? t('common.buttons.deleting') : t('common.buttons.confirmDelete') }}</AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.table-loading {
  opacity: 0.55;
  pointer-events: none;
}
</style>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppModal from '@/components/ui/AppModal.vue'
import ArtifactPreview from '@/components/run/ArtifactPreview.vue'
import { api } from '@/lib/api'
import type { Artifact } from '@/lib/types'

const props = defineProps<{
  open: boolean
  runId: string | null
  contextLabel?: string
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const { t } = useI18n()
const router = useRouter()

const loading = ref(false)
const loadError = ref<string | null>(null)
const artifacts = ref<Artifact[]>([])
const runTitle = ref('')
const activeIdx = ref(0)

const activeArtifact = computed(() => artifacts.value[activeIdx.value] ?? null)

const subtitle = computed(() => {
  const parts = [props.runId, props.contextLabel, runTitle.value].filter(Boolean)
  return parts.join(' · ')
})

function formatSize(bytes?: number): string {
  if (bytes == null || !Number.isFinite(bytes)) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function load() {
  const id = props.runId
  if (!id) {
    artifacts.value = []
    runTitle.value = ''
    loadError.value = null
    return
  }
  loading.value = true
  loadError.value = null
  activeIdx.value = 0
  try {
    const run = await api.getRun(id)
    runTitle.value = (run.title || '').trim() || run.workflowName || id
    artifacts.value = Array.isArray(run.artifacts) ? run.artifacts : []
    activeIdx.value = 0
  } catch (err) {
    artifacts.value = []
    runTitle.value = ''
    loadError.value = err instanceof Error && err.message ? err.message : String(err || 'load failed')
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.open, props.runId] as const,
  ([open]) => {
    if (open && props.runId) void load()
    if (!open) {
      artifacts.value = []
      loadError.value = null
      activeIdx.value = 0
    }
  },
  { immediate: true },
)

function selectRow(idx: number) {
  activeIdx.value = idx
}

function close() {
  emit('close')
}

function openRunDetail() {
  const id = props.runId
  close()
  if (id) void router.push(`/runs/${id}`)
}
</script>

<template>
  <AppModal
    :open="open"
    :title="t('shell.runNotifications.outputTitle')"
    :width="1120"
    :body-overflow="'hidden'"
    :body-min-height="520"
    close-on-esc
    @close="close"
  >
    <p class="mb-2 text-xs text-txt3">{{ subtitle || '—' }}</p>
    <p class="mb-3 text-xs text-txt2">{{ t('shell.runNotifications.outputHint') }}</p>

    <div v-if="loading" class="py-10 text-center text-sm text-txt2" data-testid="run-output-loading">
      {{ t('shell.runNotifications.loadingOutputs') }}
    </div>
    <div
      v-else-if="loadError"
      class="border border-err/35 bg-err/8 px-3 py-3 text-sm text-err"
      data-testid="run-output-load-error"
      role="alert"
    >
      {{ loadError }}
    </div>
    <div
      v-else-if="!artifacts.length"
      class="border border-line bg-base px-4 py-10 text-center"
      data-testid="run-output-empty"
    >
      <p class="text-sm text-txt2">{{ t('shell.runNotifications.noArtifacts') }}</p>
      <p class="mt-1.5 text-xs text-txt3">{{ t('shell.runNotifications.noArtifactsHint') }}</p>
    </div>
    <div
      v-else
      class="flex h-[min(62vh,560px)] min-h-[420px] gap-0 overflow-hidden border border-line"
      data-testid="run-output-master-detail"
    >
      <aside
        class="flex w-[280px] shrink-0 flex-col border-r border-line bg-base"
        :aria-label="t('shell.runNotifications.listAria')"
        data-testid="run-output-list"
      >
        <div class="scroll-area min-h-0 flex-1 overflow-y-auto">
          <button
            v-for="(a, idx) in artifacts"
            :key="a.id || `${a.name}-${idx}`"
            type="button"
            class="flex w-full flex-col gap-1 border-b border-line px-3 py-2.5 text-left transition-colors hover:bg-elevated"
            :class="idx === activeIdx ? 'bg-accent-dim/40 ring-1 ring-inset ring-accent' : ''"
            data-testid="run-output-row"
            :aria-selected="idx === activeIdx ? 'true' : 'false'"
            @click="selectRow(idx)"
          >
            <div class="flex items-center gap-2">
              <span class="chip shrink-0 border-n-artifact/30 text-[10px] text-n-artifact">{{
                a.kind || 'file'
              }}</span>
              <span class="min-w-0 flex-1 truncate text-[12px] font-medium text-txt" :title="a.name">{{
                a.name
              }}</span>
            </div>
            <div class="flex items-center justify-between gap-2 text-[10px] text-txt3">
              <span class="min-w-0 truncate" :title="a.nodeId">{{ a.nodeId || '—' }}</span>
              <span class="shrink-0 font-mono">{{ formatSize(a.sizeBytes) }}</span>
            </div>
          </button>
        </div>
      </aside>
      <section
        class="flex min-w-0 flex-1 flex-col overflow-hidden bg-surface"
        data-testid="run-output-preview"
      >
        <ArtifactPreview
          v-if="activeArtifact"
          :artifact="activeArtifact"
          :artifacts="artifacts"
          :run-id="runId || undefined"
          hide-delete
          hide-copy
          hide-zoom
          hide-export
        />
      </section>
    </div>

    <template #footer>
      <button
        type="button"
        class="border border-line bg-transparent px-3 py-2 text-[13px] text-txt2 hover:border-line-strong hover:text-txt"
        data-testid="run-output-open-run"
        @click="openRunDetail"
      >
        {{ t('shell.runNotifications.openRunDetail') }}
      </button>
      <button
        type="button"
        class="border border-transparent bg-accent px-3 py-2 text-[13px] text-white hover:brightness-110"
        data-testid="run-output-done"
        @click="close"
      >
        {{ t('shell.runNotifications.done') }}
      </button>
    </template>
  </AppModal>
</template>

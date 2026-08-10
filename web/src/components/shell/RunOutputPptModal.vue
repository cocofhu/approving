<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppModal from '@/components/ui/AppModal.vue'
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
)

function selectCard(idx: number) {
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

function slideNo(idx: number, total: number): string {
  const n = String(idx + 1).padStart(2, '0')
  const t = String(total).padStart(2, '0')
  return `${n} / ${t}`
}

function displayTitle(a: Artifact): string {
  const base = a.name.replace(/\.[^.]+$/, '')
  return base || a.name
}
</script>

<template>
  <AppModal
    :open="open"
    :title="t('shell.runNotifications.outputTitle')"
    :width="920"
    close-on-esc
    @close="close"
  >
    <p class="mb-3 text-xs text-txt3">{{ subtitle || '—' }}</p>
    <p class="mb-4 text-xs text-txt2">{{ t('shell.runNotifications.outputHint') }}</p>

    <div v-if="loading" class="py-10 text-center text-sm text-txt2" data-testid="run-output-loading">
      {{ t('shell.runNotifications.loadingOutputs') }}
    </div>
    <div v-else-if="loadError" class="border border-err/35 bg-err/8 px-3 py-3 text-sm text-err">
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
    <template v-else>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3" data-testid="run-output-deck">
        <button
          v-for="(a, idx) in artifacts"
          :key="a.id || `${a.name}-${idx}`"
          type="button"
          class="border border-line bg-base text-left transition-colors hover:border-accent"
          :class="idx === activeIdx ? 'border-accent ring-2 ring-accent/30' : ''"
          data-testid="run-output-card"
          @click="selectCard(idx)"
        >
          <div
            class="relative flex aspect-video flex-col justify-between overflow-hidden border-b border-line bg-gradient-to-br from-elevated via-base to-accent-dim/40 p-3.5"
          >
            <div class="relative z-[1] text-[10px] uppercase tracking-wider text-accent-2">
              {{ (a.kind || 'file').toUpperCase() }}
            </div>
            <div class="relative z-[1] line-clamp-2 text-sm font-semibold leading-snug text-txt">
              {{ displayTitle(a) }}
            </div>
            <div class="relative z-[1] font-mono text-[11px] text-txt3">
              {{ slideNo(idx, artifacts.length) }}
            </div>
          </div>
          <div class="px-3 py-2.5">
            <div class="truncate text-[13px] font-medium text-txt">{{ a.name }}</div>
            <div class="mt-1 text-[11px] text-txt3">
              <span class="inline-flex border border-line bg-elevated px-1.5 py-0.5">{{ a.kind }}</span>
              {{ t('shell.runNotifications.outputCard') }}
            </div>
          </div>
        </button>
      </div>

      <div
        v-if="activeArtifact"
        class="mt-4 border border-line bg-base"
        data-testid="run-output-preview"
      >
        <div class="flex items-center justify-between border-b border-line px-3.5 py-2.5 text-xs text-txt2">
          <span>{{ t('shell.runNotifications.previewLabel') }} · {{ activeArtifact.name }}</span>
          <span class="border border-line bg-elevated px-1.5 py-0.5 text-[10px]">16:9</span>
        </div>
        <div
          class="flex aspect-video max-h-80 flex-col justify-center gap-2.5 bg-gradient-to-br from-elevated to-base px-8 py-7"
        >
          <div class="text-[11px] uppercase tracking-wider text-accent-2">
            {{ (activeArtifact.kind || 'file').toUpperCase() }} · SLIDE
            {{ String(activeIdx + 1).padStart(2, '0') }}
          </div>
          <div class="text-[22px] font-semibold tracking-tight text-txt">
            {{ displayTitle(activeArtifact) }}
          </div>
          <div class="max-w-xl text-[13px] leading-relaxed text-txt2">
            {{
              t('shell.runNotifications.previewSummary', {
                kind: activeArtifact.kind,
                name: activeArtifact.name,
              })
            }}
          </div>
        </div>
      </div>
    </template>

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

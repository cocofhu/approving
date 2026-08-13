<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppModal from '@/components/ui/AppModal.vue'
import OutputResultCards from '@/components/run/OutputResultCards.vue'
import { api } from '@/lib/api/api'
import { resolveOutputFocusNodeId } from '@/lib/run/runOutputSelection'
import { resolveNodeDisplayLabelFromNode } from '@/lib/run/resolveNodeDisplayLabel'
import type { OutputCard, Run, WFNode } from '@/lib/shared/types'

const props = defineProps<{
  open: boolean
  runId: string | null
  contextLabel?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'mark-read'): void
}>()

const { t } = useI18n()
const router = useRouter()

const loading = ref(false)
const loadError = ref<string | null>(null)
const run = ref<Run | null>(null)
const runTitle = ref('')
const focusNodeId = ref<string | null>(null)
const focusNode = ref<WFNode | null>(null)
const outputCards = ref<OutputCard[]>([])

const subtitle = computed(() => {
  const parts = [props.runId, props.contextLabel, runTitle.value].filter(Boolean)
  return parts.join(' · ')
})

const focusNodeLabel = computed(() => {
  if (!focusNode.value) return ''
  return resolveNodeDisplayLabelFromNode(focusNode.value, t)
})

const hasCards = computed(() => outputCards.value.length > 0)

function graphNodes(r: Run): WFNode[] {
  return Array.isArray(r.nodes) ? r.nodes : []
}

function cardsForFocus(r: Run, nodeId: string | null): OutputCard[] {
  if (!nodeId) return []
  const nr = r.nodeRuns?.[nodeId]
  const raw = nr?.outputs?.outputCards
  return Array.isArray(raw) ? (raw as OutputCard[]) : []
}

function resetState() {
  run.value = null
  runTitle.value = ''
  focusNodeId.value = null
  focusNode.value = null
  outputCards.value = []
  loadError.value = null
}

async function load() {
  const id = props.runId
  if (!id) {
    resetState()
    return
  }
  loading.value = true
  loadError.value = null
  try {
    const loaded = await api.getRun(id)
    run.value = loaded
    runTitle.value = (loaded.title || '').trim() || loaded.workflowName || id
    const nodes = graphNodes(loaded)
    const focusId = resolveOutputFocusNodeId(loaded, nodes)
    focusNodeId.value = focusId
    focusNode.value = focusId ? nodes.find((n) => n.id === focusId) ?? null : null
    outputCards.value = cardsForFocus(loaded, focusId)
  } catch (err) {
    resetState()
    loadError.value = err instanceof Error && err.message ? err.message : String(err || 'load failed')
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.open, props.runId] as const,
  ([open]) => {
    if (open && props.runId) void load()
    if (!open) resetState()
  },
  { immediate: true },
)

function close() {
  emit('close')
}

function markRead() {
  emit('mark-read')
}

function openRunDetail() {
  const id = props.runId
  close()
  if (!id) return
  const query: Record<string, string> = {}
  if (focusNodeId.value) {
    query.node = focusNodeId.value
    query.tab = 'output'
  }
  void router.push({ path: `/runs/${id}`, query })
}

function openArtifacts() {
  const id = props.runId
  close()
  if (!id) return
  void router.push({ path: `/runs/${id}`, query: { detail: 'artifacts' } })
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
      v-else-if="!hasCards"
      class="flex min-h-[360px] flex-col items-center justify-center border border-line bg-base px-4 py-10 text-center"
      data-testid="run-output-empty"
    >
      <strong class="mb-1.5 block text-[13px] text-txt">{{
        t('shell.runNotifications.noFinalResults')
      }}</strong>
      <p class="max-w-md text-sm text-txt2">{{ t('shell.runNotifications.noFinalResultsHint') }}</p>
      <p class="mt-1.5 max-w-md text-xs text-txt3">{{ t('shell.runNotifications.noFinalResultsNote') }}</p>
      <div class="mt-4 flex flex-wrap items-center justify-center gap-2">
        <button
          type="button"
          class="border border-transparent bg-accent px-3 py-2 text-[13px] text-white hover:brightness-110"
          data-testid="run-output-empty-open-run"
          @click="openRunDetail"
        >
          {{ t('shell.runNotifications.openRunDetail') }}
        </button>
        <button
          type="button"
          class="border border-line bg-transparent px-3 py-2 text-[13px] text-txt2 hover:border-line-strong hover:text-txt"
          data-testid="run-output-empty-open-artifacts"
          @click="openArtifacts"
        >
          {{ t('shell.runNotifications.viewArtifacts') }}
        </button>
      </div>
    </div>
    <div
      v-else
      class="flex h-[min(62vh,560px)] min-h-[420px] flex-col overflow-hidden border border-line"
      data-testid="run-output-result-cards"
    >
      <div
        class="flex shrink-0 flex-wrap items-center gap-2 border-b border-line bg-elevated px-3 py-2 text-[12px] text-txt2"
        data-testid="run-output-focus-bar"
      >
        <span>{{ t('shell.runNotifications.focusOutputPrefix') }}</span>
        <strong class="text-txt">{{ focusNodeLabel || focusNodeId || '—' }}</strong>
        <span class="border border-ok/35 bg-ok/8 px-2 py-0.5 text-[11px] text-ok">{{
          t('shell.runNotifications.focusOutputType')
        }}</span>
        <span class="border border-line bg-base px-2 py-0.5 text-[11px] text-txt2">{{
          t('shell.runNotifications.focusOutputSource')
        }}</span>
        <span class="border border-line bg-base px-2 py-0.5 text-[11px] text-txt2">{{
          t('shell.runNotifications.focusOutputAligned')
        }}</span>
      </div>
      <div class="scroll-area min-h-0 flex-1 overflow-y-auto bg-surface px-4 py-3">
        <OutputResultCards v-if="run" :cards="outputCards" :run="run" />
      </div>
    </div>

    <template #footer>
      <button
        type="button"
        class="border border-transparent bg-accent px-3 py-2 text-[13px] text-white hover:brightness-110"
        data-testid="run-output-mark-read"
        @click="markRead"
      >
        {{ t('shell.runNotifications.markAsRead') }}
      </button>
    </template>
  </AppModal>
</template>

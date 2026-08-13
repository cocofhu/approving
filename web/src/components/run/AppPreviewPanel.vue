<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, type PreviewPort } from '@/lib/api/api'
import type { AppPreviewPickPayload } from '@/lib/shared/previewPickUrl'
import NovncPreviewPanel from './NovncPreviewPanel.vue'
import DirectPreviewFrame from './DirectPreviewFrame.vue'
import PreviewFeedbackChat from './PreviewFeedbackChat.vue'
import RefreshStrip from './RefreshStrip.vue'
import HardLoadLayer from './HardLoadLayer.vue'
import { isAbortError } from '@/lib/run/liveLogRehydrate'

const props = withDefaults(
  defineProps<{
    runId: string
    nodeId: string
    compact?: boolean
    fill?: boolean
    /** When false, hide PreviewFeedbackChat (review ReAct is the primary dialogue). */
    showFeedback?: boolean
    /** Gates Inbox: show share-approval on noVNC toolbar. */
    shareEnabled?: boolean
  }>(),
  { compact: false, fill: false, showFeedback: true, shareEnabled: false },
)

const emit = defineEmits<{
  (e: 'pick', payload: AppPreviewPickPayload): void
  (e: 'staged-pick', payload: AppPreviewPickPayload | null): void
  (e: 'issues-changed'): void
  (e: 'open-share'): void
}>()

const { t } = useI18n()
const ports = ref<PreviewPort[]>([])
const loading = ref(false)
const loadError = ref<string | null>(null)
const activePort = ref<number | null>(null)
const pickedSelector = ref('')
let portsGen = 0
let portsAbort: AbortController | null = null

/** API Tab uses PreviewProxy iframe; all other ports use noVNC. */
function isApiPort(p: PreviewPort): boolean {
  const label = (p.label || '').trim().toLowerCase()
  return label.includes('api')
}

function isDirectPort(p: PreviewPort): boolean {
  return p.mode === 'direct' && !!(p.directUrl || '').trim()
}

function onPick(payload: AppPreviewPickPayload) {
  pickedSelector.value = payload.selector
  emit('pick', payload)
}

function onStagedPick(payload: AppPreviewPickPayload | null) {
  if (payload) pickedSelector.value = payload.selector
  emit('staged-pick', payload)
}

async function loadPorts() {
  portsAbort?.abort()
  const gen = ++portsGen
  portsAbort = new AbortController()
  loading.value = true
  loadError.value = null
  try {
    const r = await api.nodePreviews(props.runId, props.nodeId, { signal: portsAbort.signal })
    if (gen !== portsGen) return
    ports.value = r.ports || []
    if (ports.value.length && activePort.value == null) {
      activePort.value = ports.value[0].port
    } else if (activePort.value != null && !ports.value.some((p) => p.port === activePort.value)) {
      activePort.value = ports.value[0]?.port ?? null
    }
  } catch (e: any) {
    if (gen !== portsGen || isAbortError(e) || portsAbort.signal.aborted) return
    loadError.value = t('pages.appPreview.loadFailed')
    if (!ports.value.length) ports.value = []
  } finally {
    if (gen === portsGen) loading.value = false
  }
}

watch(() => [props.runId, props.nodeId], loadPorts, { immediate: true })

onUnmounted(() => {
  portsAbort?.abort()
  portsAbort = null
  portsGen++
})

function tabLabel(p: PreviewPort): string {
  return (p.label || '').trim() || String(p.port)
}

function selectPort(port: number) {
  activePort.value = port
}
</script>

<template>
  <div
    data-testid="app-preview"
    class="relative flex min-h-0 flex-col"
    :class="fill ? 'h-full flex-1' : ''"
    :aria-busy="loading ? 'true' : 'false'"
  >
    <RefreshStrip v-if="loading && ports.length" />
    <HardLoadLayer
      v-else-if="loading && !ports.length && !loadError"
      :overlay="false"
      :stuck-after-ms="10_000"
      :stage="t('pages.appPreview.loading')"
      @retry="loadPorts"
    />
    <div
      v-if="loadError"
      class="rounded-md border border-err/30 bg-err/10 p-4 text-xs text-err"
      role="alert"
      data-testid="app-preview-load-error"
    >
      <p>{{ loadError }}</p>
      <button
        type="button"
        class="mt-2 inline-flex min-h-11 items-center border border-line px-3 text-[12px] text-txt"
        @click="loadPorts"
      >
        {{ t('common.chatImage.retry') }}
      </button>
    </div>
    <div
      v-else-if="!loading && !ports.length"
      class="rounded-md border border-line bg-elevated p-4 text-xs text-txt3"
      data-testid="app-preview-empty"
    >
      {{ t('pages.appPreview.noPorts') }}
    </div>
    <template v-if="ports.length">
      <div v-if="ports.length > 1" class="mb-2 flex flex-wrap gap-1">
        <button
          v-for="p in ports"
          :key="p.port"
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition"
          :class="
            p.port === activePort
              ? 'bg-accent/15 text-accent'
              : 'border border-line text-txt2 hover:bg-elevated hover:text-txt'
          "
          @click="selectPort(p.port)"
        >
          {{ tabLabel(p) }}
        </button>
      </div>
      <div
        class="flex min-h-0 flex-col overflow-hidden rounded-md border border-line bg-surface"
        :class="fill ? 'flex-1' : compact ? 'h-[280px]' : 'h-[420px]'"
      >
        <DirectPreviewFrame
          v-for="p in ports.filter((x) => isDirectPort(x))"
          v-show="activePort === p.port"
          :key="`direct-${p.port}`"
          :direct-url="p.directUrl || ''"
          :title="tabLabel(p)"
          @pick="onPick"
          @staged-pick="onStagedPick"
        />
        <keep-alive :max="ports.length">
          <NovncPreviewPanel
            v-for="p in ports.filter((x) => !isApiPort(x) && !isDirectPort(x))"
            v-show="activePort === p.port"
            :key="`vnc-${p.port}`"
            :run-id="runId"
            :node-id="nodeId"
            :port="p.port"
            fill
            :compact="compact"
            :share-enabled="shareEnabled"
            @pick="onPick"
            @staged-pick="onStagedPick"
            @open-share="emit('open-share')"
          />
        </keep-alive>
        <iframe
          v-for="p in ports.filter((x) => isApiPort(x) && !isDirectPort(x))"
          v-show="activePort === p.port"
          :key="`api-${p.port}`"
          :src="p.proxyUrl"
          class="h-full w-full border-0 bg-base"
          :title="tabLabel(p)"
        />
      </div>
      <PreviewFeedbackChat
        v-if="!compact && showFeedback"
        :run-id="runId"
        :node-id="nodeId"
        :port="activePort ?? 0"
        :selector="pickedSelector"
        copy-variant="review"
        :compact="fill"
        @clear-selector="pickedSelector = ''"
        @issues-changed="emit('issues-changed')"
      />
    </template>
  </div>
</template>

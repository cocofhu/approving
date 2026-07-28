<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, type PreviewPort } from '@/lib/api'
import NovncPreviewPanel from './NovncPreviewPanel.vue'
import PreviewFeedbackChat from './PreviewFeedbackChat.vue'

const props = withDefaults(
  defineProps<{
    runId: string
    nodeId: string
    compact?: boolean
    fill?: boolean
    /** When false, hide PreviewFeedbackChat (review ReAct is the primary dialogue). */
    showFeedback?: boolean
  }>(),
  { compact: false, fill: false, showFeedback: true },
)

const emit = defineEmits<{
  (e: 'pick', payload: { selector: string; tagName: string; outerHTML: string }): void
  (e: 'staged-pick', payload: { selector: string; tagName: string; outerHTML: string } | null): void
  (e: 'issues-changed'): void
}>()

const { t } = useI18n()
const ports = ref<PreviewPort[]>([])
const loading = ref(false)
const loadError = ref<string | null>(null)
const activePort = ref<number | null>(null)
const pickedSelector = ref('')

/** API Tab uses PreviewProxy iframe; all other ports use noVNC. */
function isApiPort(p: PreviewPort): boolean {
  const label = (p.label || '').trim().toLowerCase()
  return label.includes('api')
}

function onPick(payload: { selector: string; tagName: string; outerHTML: string }) {
  pickedSelector.value = payload.selector
  emit('pick', payload)
}

function onStagedPick(payload: { selector: string; tagName: string; outerHTML: string } | null) {
  if (payload) pickedSelector.value = payload.selector
  emit('staged-pick', payload)
}

async function loadPorts() {
  loading.value = true
  loadError.value = null
  try {
    const r = await api.nodePreviews(props.runId, props.nodeId)
    ports.value = r.ports || []
    if (ports.value.length && activePort.value == null) {
      activePort.value = ports.value[0].port
    } else if (activePort.value != null && !ports.value.some((p) => p.port === activePort.value)) {
      activePort.value = ports.value[0]?.port ?? null
    }
  } catch (e: any) {
    loadError.value = e?.message || 'load failed'
    ports.value = []
  } finally {
    loading.value = false
  }
}

watch(() => [props.runId, props.nodeId], loadPorts, { immediate: true })

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
    class="flex min-h-0 flex-col"
    :class="fill ? 'h-full flex-1' : ''"
  >
    <div v-if="loading" class="p-4 text-xs text-txt3">{{ t('pages.appPreview.loading') }}</div>
    <div v-else-if="loadError" class="rounded-md border border-err/30 bg-err/10 p-4 text-xs text-err">
      {{ loadError }}
    </div>
    <div v-else-if="!ports.length" class="rounded-md border border-line bg-elevated p-4 text-xs text-txt3">
      {{ t('pages.appPreview.noPorts') }}
    </div>
    <template v-else>
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
        <keep-alive :max="ports.length">
          <NovncPreviewPanel
            v-for="p in ports.filter((x) => !isApiPort(x))"
            v-show="activePort === p.port"
            :key="`vnc-${p.port}`"
            :run-id="runId"
            :node-id="nodeId"
            :port="p.port"
            fill
            :compact="compact"
            @pick="onPick"
            @staged-pick="onStagedPick"
          />
        </keep-alive>
        <iframe
          v-for="p in ports.filter((x) => isApiPort(x))"
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

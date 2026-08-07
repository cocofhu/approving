<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { api, type LiveDecisionSample, type LiveTraceSpan } from '@/lib/api'
import { relTime } from '@/lib/format'
import { useToast } from '@/lib/useToast'

const props = defineProps<{
  projectId: string
  /** Prefill when opening from a channel thread. */
  initialConversationId?: string
}>()

const { t } = useI18n()
const toast = useToast()

const items = ref<LiveDecisionSample[]>([])
const loading = ref(true)
const conversationId = ref((props.initialConversationId || '').trim())
const traceId = ref('')
const openId = ref('')

watch(
  () => props.initialConversationId,
  (v) => {
    if (v && !conversationId.value) conversationId.value = v.trim()
  },
)

function parseSpans(raw: string | undefined): LiveTraceSpan[] {
  if (!raw?.trim()) return []
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed.filter((x): x is LiveTraceSpan => !!x && typeof x === 'object' && typeof (x as LiveTraceSpan).name === 'string')
  } catch {
    return []
  }
}

function previewText(text: string, max = 120) {
  const s = (text || '').replace(/\s+/g, ' ').trim()
  if (s.length <= max) return s
  return s.slice(0, max) + '…'
}

function toggle(id: string) {
  openId.value = openId.value === id ? '' : id
}

async function load() {
  loading.value = true
  try {
    const res = await api.listLiveTraces(props.projectId, {
      conversationId: conversationId.value.trim() || undefined,
      traceId: traceId.value.trim() || undefined,
      limit: 50,
    })
    items.value = res.items ?? []
    if (items.value.length === 1) openId.value = items.value[0].id
  } catch (e) {
    toast.error(e instanceof Error ? e.message : t('pages.projectDetail.pm.tracesLoadFailed'))
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-3 pb-4" data-testid="pm-live-traces-panel">
    <div class="shrink-0 space-y-1">
      <h2 class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.pm.tracesTitle') }}</h2>
      <p class="text-xs text-txt2">{{ t('pages.projectDetail.pm.tracesHint') }}</p>
    </div>

    <form
      class="flex shrink-0 flex-col gap-2 sm:flex-row sm:items-end"
      data-testid="pm-live-traces-filters"
      @submit.prevent="load"
    >
      <label class="min-w-0 flex-1 space-y-1">
        <span class="text-[11px] text-txt3">{{ t('pages.projectDetail.pm.tracesFilterConversation') }}</span>
        <input
          v-model="conversationId"
          type="text"
          class="input w-full font-mono text-xs"
          data-testid="pm-live-traces-conversation"
          :placeholder="t('pages.projectDetail.pm.tracesFilterConversationPh')"
        >
      </label>
      <label class="min-w-0 flex-1 space-y-1">
        <span class="text-[11px] text-txt3">{{ t('pages.projectDetail.pm.tracesFilterTrace') }}</span>
        <input
          v-model="traceId"
          type="text"
          class="input w-full font-mono text-xs"
          data-testid="pm-live-traces-trace-id"
          :placeholder="t('pages.projectDetail.pm.tracesFilterTracePh')"
        >
      </label>
      <AppButton type="submit" size="sm" variant="outline" data-testid="pm-live-traces-search" :disabled="loading">
        {{ t('pages.projectDetail.pm.tracesSearch') }}
      </AppButton>
    </form>

    <div v-if="loading" class="py-8 text-center text-sm text-txt2" data-testid="pm-live-traces-loading">
      {{ t('pages.projectDetail.pm.tracesLoading') }}
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      data-testid="pm-live-traces-empty"
      :title="t('pages.projectDetail.pm.tracesEmpty')"
      :desc="t('pages.projectDetail.pm.tracesEmptyHint')"
    />

    <ul v-else class="scroll-area min-h-0 flex-1 space-y-2 overflow-y-auto" data-testid="pm-live-traces-list">
      <li
        v-for="sample in items"
        :key="sample.id"
        class="border border-line bg-surface"
        :data-testid="`pm-live-trace-${sample.id}`"
      >
        <button
          type="button"
          class="flex w-full items-start gap-3 px-3 py-2.5 text-left hover:bg-elevated/60"
          :data-testid="`pm-live-trace-toggle-${sample.id}`"
          @click="toggle(sample.id)"
        >
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-mono text-[11px] text-txt2">{{ sample.traceId || sample.id }}</span>
              <span
                v-if="sample.route"
                class="rounded border border-line px-1 text-[10px] uppercase tracking-wide text-txt2"
              >{{ sample.route }}</span>
              <span
                v-if="sample.degraded"
                class="rounded border border-amber-500/40 bg-amber-500/10 px-1 text-[10px] text-amber-700 dark:text-amber-300"
              >{{ t('pages.projectDetail.pm.tracesDegraded') }}</span>
              <span v-if="sample.latencyMs > 0" class="text-[10px] tabular-nums text-txt3">
                {{ sample.latencyMs }}ms
              </span>
            </div>
            <p class="line-clamp-2 text-sm text-txt">{{ previewText(sample.userText) || '—' }}</p>
            <p class="text-[11px] text-txt3">
              <span v-if="sample.conversationId">
                {{ t('pages.projectDetail.pm.tasksConversation') }}: {{ sample.conversationId }}
                ·
              </span>
              <span v-if="sample.model">{{ sample.model }} · </span>
              {{ relTime(sample.createdAt) }}
            </p>
          </div>
          <span class="shrink-0 text-txt3">{{ openId === sample.id ? '▾' : '▸' }}</span>
        </button>

        <div
          v-if="openId === sample.id"
          class="space-y-3 border-t border-line px-3 py-3"
          :data-testid="`pm-live-trace-detail-${sample.id}`"
        >
          <div v-if="parseSpans(sample.spans).length" class="space-y-1.5">
            <div class="text-[11px] font-medium uppercase tracking-wide text-txt3">
              {{ t('pages.projectDetail.pm.tracesSpans') }}
            </div>
            <ol class="space-y-1" data-testid="pm-live-trace-spans">
              <li
                v-for="(span, i) in parseSpans(sample.spans)"
                :key="`${span.name}-${i}`"
                class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 font-mono text-[11px]"
              >
                <span class="text-txt">{{ span.name }}</span>
                <span
                  class="rounded px-1 text-[10px] uppercase"
                  :class="span.status === 'ok' || span.status === 'skipped'
                    ? 'text-txt2'
                    : 'text-amber-700 dark:text-amber-300'"
                >{{ span.status || '—' }}</span>
                <span v-if="span.durationMs != null" class="tabular-nums text-txt3">{{ span.durationMs }}ms</span>
                <span v-if="span.detail" class="min-w-0 break-all text-txt2">{{ span.detail }}</span>
              </li>
            </ol>
          </div>
          <div v-else class="text-xs text-txt3">{{ t('pages.projectDetail.pm.tracesNoSpans') }}</div>

          <div v-if="sample.egress || sample.pmOutcome" class="space-y-1 text-xs">
            <p v-if="sample.egress">
              <span class="text-txt3">{{ t('pages.projectDetail.pm.tracesEgress') }}:</span>
              {{ sample.egress }}
            </p>
            <p v-if="sample.pmOutcome" class="whitespace-pre-wrap break-words text-txt2">
              <span class="text-txt3">{{ t('pages.projectDetail.pm.tracesOutcome') }}:</span>
              {{ previewText(sample.pmOutcome, 400) }}
            </p>
          </div>

          <details v-if="sample.actions" class="text-xs">
            <summary class="cursor-pointer text-txt2">{{ t('pages.projectDetail.pm.tracesActions') }}</summary>
            <pre class="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] text-txt2">{{ sample.actions }}</pre>
          </details>
          <details v-if="sample.toolResults" class="text-xs">
            <summary class="cursor-pointer text-txt2">{{ t('pages.projectDetail.pm.tracesTools') }}</summary>
            <pre class="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] text-txt2">{{ sample.toolResults }}</pre>
          </details>
          <details v-if="sample.rawCompletion" class="text-xs">
            <summary class="cursor-pointer text-txt2">{{ t('pages.projectDetail.pm.tracesRaw') }}</summary>
            <pre class="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] text-txt2">{{ sample.rawCompletion }}</pre>
          </details>
        </div>
      </li>
    </ul>

    <div class="shrink-0">
      <AppButton size="sm" variant="ghost" data-testid="pm-live-traces-refresh" :disabled="loading" @click="load">
        {{ t('pages.projectDetail.pm.tracesRefresh') }}
      </AppButton>
    </div>
  </div>
</template>

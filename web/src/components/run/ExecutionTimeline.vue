<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import StatusPill from '../ui/StatusPill.vue'
import VarValueDisplay from '../ui/VarValueDisplay.vue'
import TruncatedTextTooltip from '../ui/TruncatedTextTooltip.vue'
import { fmtTime, fmtDuration } from '@/lib/format'
import { formatVarChip, formatVarValue } from '@/lib/compositeText'
import { NODE_DEFS, nodeColorHex } from '@/data/nodeRegistry'
import { resolveNodeDisplayLabel } from '@/lib/resolveNodeDisplayLabel'
import { compareTimelineOrder, resolveProcessDuration } from '@/lib/runStats'
import type { NodeRun, NodeRunStatus, Run, WFNode } from '@/lib/types'

const props = withDefaults(
  defineProps<{
    run: Run
    nodes: WFNode[]
    selectedNodeId: string | null
    // Array index of the selected execution within its node's history
    // (aligns with RunDetailView's selExecIdx / selIterIdx).
    selectedExecIdx: number
    /** When false, rows are not clickable and never show selection highlight (stats mode). */
    interactive?: boolean
    /** Optional shared clock from parent (stats mode); falls back to local 1s tick. */
    nowMs?: number
  }>(),
  { interactive: true },
)
const emit = defineEmits<{ (e: 'select', nodeId: string, idx: number): void }>()

const { t, locale } = useI18n()

/** Currently expanded var chip key (`nodeId:idx:varName`); null = none. */
const expandedKey = ref<string | null>(null)

function varExpandKey(it: TimelineItem, k: string) {
  return `${it.nodeId}:${it.idxInNode}:${k}`
}

function toggleVarExpand(it: TimelineItem, k: string, ev?: Event) {
  ev?.stopPropagation()
  const key = varExpandKey(it, k)
  expandedKey.value = expandedKey.value === key ? null : key
}

function expandedVar(it: TimelineItem): { k: string; v: unknown } | null {
  if (!expandedKey.value) return null
  const prefix = `${it.nodeId}:${it.idxInNode}:`
  if (!expandedKey.value.startsWith(prefix)) return null
  const k = expandedKey.value.slice(prefix.length)
  return it.vars.find((e) => e.k === k) ?? null
}

function fmtVarFull(v: unknown): string {
  if (v != null && typeof v === 'object') {
    try {
      return JSON.stringify(v, null, 2)
    } catch {
      return String(v)
    }
  }
  return formatVarValue(v)
}

interface TimelineItem {
  nodeId: string
  idxInNode: number
  iteration: number
  status: NodeRunStatus
  startedAt?: string
  durationSec?: number
  label: string
  type: string
  vars: { k: string; v: unknown }[]
  mcp: { tool: string; isError: boolean; tip: string }[]
}

// Kept for chip title tooltips (plain text).
function fmtVar(v: any): string {
  return formatVarChip(v)
}

function varEntries(snap?: Record<string, any>): { k: string; v: unknown }[] {
  if (!snap) return []
  return Object.keys(snap)
    .sort()
    .map((k) => ({ k, v: snap[k] }))
}

// Map an execution's MCP tool calls into compact chips (tool name + ok/err),
// with a hover tip showing truncated in/out for quick debugging.
function mcpEntries(calls?: NodeRun['mcpCalls']): { tool: string; isError: boolean; tip: string }[] {
  if (!calls) return []
  return calls.map((c) => ({
    tool: c.tool,
    isError: !!c.isError,
    tip: `${c.tool}\n${t('pages.liveLog.args')} ${c.args || t('common.emptyParen')}\n${t('pages.liveLog.result')} ${c.result || t('common.emptyParen')}`,
  }))
}

const nodeById = computed<Record<string, WFNode>>(() => {
  const m: Record<string, WFNode> = {}
  for (const n of props.nodes) m[n.id] = n
  return m
})

// Flatten every node's execution history into a single chronological list. The
// backend keeps one StateRun row per execution (loop-back / gate revise / retry
// each add a row), so this reads as a linear "what happened, in order" trace.
// Prefer nodeExecutions; fall back to nodeRuns for live nodes missing history
// (same boundary as runStats.flattenProcesses).
const items = computed<TimelineItem[]>(() => {
  void locale.value
  const execs = props.run.nodeExecutions || {}
  const nodeRuns = props.run.nodeRuns || {}
  const out: TimelineItem[] = []
  const ids = new Set([...Object.keys(execs), ...Object.keys(nodeRuns)])
  for (const nodeId of ids) {
    const list = execs[nodeId]?.length
      ? execs[nodeId]!
      : nodeRuns[nodeId]
        ? [nodeRuns[nodeId]!]
        : []
    const node = nodeById.value[nodeId]
    list.forEach((ex: NodeRun, idx: number) => {
      out.push({
        nodeId,
        idxInNode: idx,
        iteration: ex.iteration || idx + 1,
        status: ex.status,
        startedAt: ex.startedAt,
        durationSec: ex.durationSec,
        label: node
          ? resolveNodeDisplayLabel(node.label, node.type, t, { nodeId })
          : nodeId,
        type: node?.type || 'agent',
        vars: varEntries(ex.varsSnapshot),
        mcp: mcpEntries(ex.mcpCalls),
      })
    })
  }
  // Sort by start time; executions without a timestamp (not yet started) sink to
  // the bottom so the observed order stays chronological. Shared with runStats.
  out.sort(compareTimelineOrder)
  return out
})

// Live-ticking elapsed time for active executions. Prefer parent-shared nowMs
// (stats mode) so timeline and ExecutionStatsPanel stay in sync.
const localNowMs = ref(Date.now())
let clock: number | undefined
const hasActiveItems = computed(() =>
  items.value.some((it) => it.status === 'running' || it.status === 'waiting_human'),
)
const tickMs = computed(() => props.nowMs ?? localNowMs.value)
onMounted(() => {
  clock = window.setInterval(() => {
    if (props.nowMs != null) return
    if (hasActiveItems.value) localNowMs.value = Date.now()
  }, 1000)
})
onUnmounted(() => {
  if (clock) window.clearInterval(clock)
})

function displayDuration(it: TimelineItem): number | null {
  if (it.status === 'running' || it.status === 'waiting_human') {
    if (!it.startedAt) return null
    return resolveProcessDuration(it, tickMs.value)
  }
  if (it.status === 'completed' || it.status === 'failed' || it.status === 'skipped' || it.status === 'cancelled') {
    return it.durationSec != null ? it.durationSec : null
  }
  return null
}

function iconOf(type: string) {
  return NODE_DEFS[type as keyof typeof NODE_DEFS]?.icon || 'agent'
}
function isSelected(it: TimelineItem) {
  if (!props.interactive) return false
  return it.nodeId === props.selectedNodeId && it.idxInNode === props.selectedExecIdx
}

function onSelect(it: TimelineItem) {
  if (!props.interactive) return
  emit('select', it.nodeId, it.idxInNode)
}
// A colored dot on the connector: mirrors the execution's status so the run
// reads at a glance (running/done/failed/paused).
const DOT: Record<string, string> = {
  running: 'bg-info',
  waiting_human: 'bg-warn',
  completed: 'bg-ok',
  skipped: 'bg-txt3',
  failed: 'bg-err',
  pending: 'bg-line',
}
</script>

<template>
  <div class="flex h-full min-w-0 w-full max-w-full flex-col bg-base">
    <div class="shrink-0 border-b border-line px-4 py-2.5 text-[12px] text-txt3">
      {{ t('pages.executionTimeline.header', { n: items.length }) }}
    </div>
    <div v-if="!items.length" class="flex flex-1 items-center justify-center text-[12px] text-txt3">
      {{ t('pages.executionTimeline.empty') }}
    </div>
    <div v-else class="scroll-area safe-area-bottom min-h-0 min-w-0 w-full max-w-full flex-1 overflow-x-clip overflow-y-auto px-5 py-4">
      <ol class="relative min-w-0 max-w-full space-y-2.5 border-l border-line pl-6">
        <li v-for="(it, i) in items" :key="`${it.nodeId}:${it.idxInNode}`" class="relative min-w-0 max-w-full">
          <span
            class="absolute -left-[31px] top-4 h-2.5 w-2.5 rounded-full ring-4 ring-base"
            :class="DOT[it.status] || DOT.pending"
          />
          <div
            class="w-full min-w-0 overflow-hidden rounded-lg border bg-surface px-3 py-2.5 text-left transition-colors"
            :class="[
              isSelected(it) ? 'border-accent/60 ring-1 ring-accent/40' : 'border-line',
              interactive ? 'cursor-pointer hover:bg-elevated' : 'cursor-default',
            ]"
            @click="onSelect(it)"
          >
            <div class="flex min-w-0 items-center gap-2.5">
              <span
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md"
                :style="{ background: nodeColorHex(it.type as any) + '22', color: nodeColorHex(it.type as any) }"
              >
                <Icon :name="iconOf(it.type)" :size="16" />
              </span>
              <div class="min-w-0 flex-1 overflow-hidden">
                <div class="flex items-center gap-1.5">
                  <TruncatedTextTooltip
                    :text="it.label"
                    class="min-w-0 truncate text-[13px] font-semibold text-txt"
                    data-testid="timeline-node-label"
                  />
                  <span
                    v-if="it.iteration > 1"
                    class="shrink-0 rounded-full border border-warn/40 bg-warn/10 px-1.5 py-px text-[10px] text-warn"
                    :title="t('pages.executionTimeline.iterationTitle')"
                    >{{ t('common.iterationN', { n: it.iteration }) }}</span
                  >
                </div>
                <div v-if="displayDuration(it) != null" class="mt-0.5 text-[11px] text-txt3">
                  {{ t('pages.executionTimeline.duration', { time: fmtDuration(displayDuration(it)!) }) }}
                </div>
                <div v-if="it.vars.length" class="mt-1.5 min-w-0">
                  <div class="flex max-w-full flex-wrap gap-1 overflow-hidden">
                    <button
                      v-for="e in it.vars"
                      :key="e.k"
                      type="button"
                      data-testid="timeline-variable-chip"
                      class="inline-flex max-w-full min-w-0 items-center gap-1 overflow-hidden rounded border border-line bg-elevated px-1.5 py-px font-mono text-[10px] leading-4"
                      :class="
                        expandedKey === varExpandKey(it, e.k)
                          ? 'cursor-pointer border-accent/50 ring-1 ring-accent/30'
                          : 'cursor-pointer active:bg-surface'
                      "
                      @click="toggleVarExpand(it, e.k, $event)"
                    >
                      <span class="shrink-0 text-accent-2">{{ e.k }}</span>
                      <span class="shrink-0 text-txt3">=</span>
                      <TruncatedTextTooltip
                        :text="`${e.k} = ${fmtVar(e.v)}`"
                        :focusable="false"
                        focus-parent
                        measure-child
                        data-testid="timeline-variable-value"
                        class="min-w-0 flex-1 overflow-hidden truncate text-txt2"
                      >
                        <VarValueDisplay :value="e.v" compact class="min-w-0 max-w-full overflow-hidden truncate" />
                      </TruncatedTextTooltip>
                    </button>
                  </div>
                  <pre
                    v-if="expandedVar(it)"
                    class="scroll-area mt-1.5 max-h-40 min-w-0 max-w-full overflow-x-auto overflow-y-auto whitespace-pre rounded border border-line bg-elevated px-2 py-1.5 font-mono text-[10px] leading-4 text-txt2"
                  >{{ expandedVar(it)!.k }} = {{ fmtVarFull(expandedVar(it)!.v) }}</pre>
                </div>
                <div v-if="it.mcp.length" class="mt-1.5 flex max-w-full flex-wrap gap-1 overflow-hidden">
                  <span
                    v-for="(m, mi) in it.mcp"
                    :key="mi"
                    class="inline-flex max-w-full items-center gap-1 overflow-hidden rounded border px-1.5 py-px font-mono text-[10px] leading-4"
                    :class="m.isError ? 'border-err/50 bg-err/10 text-err' : 'border-info/40 bg-info/10 text-info'"
                  >
                    <Icon name="terminal" :size="10" />
                    <TruncatedTextTooltip :text="m.tip" class="min-w-0 truncate">{{ m.tool }}</TruncatedTextTooltip>
                    <span>{{ m.isError ? '✗' : '✓' }}</span>
                  </span>
                </div>
              </div>
              <div class="flex shrink-0 flex-col items-end gap-1">
                <StatusPill :status="it.status" size="sm" />
                <span v-if="it.startedAt" class="text-[10px] text-txt3">{{ fmtTime(it.startedAt) }}</span>
              </div>
            </div>
          </div>
        </li>
      </ol>
    </div>
  </div>
</template>

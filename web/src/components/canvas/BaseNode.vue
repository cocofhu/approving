<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Handle, Position } from '@vue-flow/core'
import Icon from '../ui/Icon.vue'
import { nodeColorHex } from '@/data/nodeRegistry'
import { useNodeDefs } from '@/lib/run/useNodeDefs'
import { resolveNodeDisplayLabel } from '@/lib/run/resolveNodeDisplayLabel'
import type { NodeType, NodeRunStatus } from '@/lib/shared/types'

const { t, locale } = useI18n()
const { NODE_DEFS } = useNodeDefs()

const props = defineProps<{
  id: string
  data: {
    type: NodeType
    label: string
    status?: NodeRunStatus
    checkpoint?: boolean
    branches?: { id: string; label: string; isDefault?: boolean }[]
    /** Legacy: action handles (prefer structuredExits for human_gate). */
    gateActions?: { id: string; label: string }[]
    /** app_preview pure ReAct review: badge + single success exit (no action handles). */
    appPreviewReview?: boolean
    structuredExits?: { id: string; label: string; tone: 'ok' | 'bad' }[]
  }
  selected?: boolean
}>()

const isBranch = computed(() => !!props.data.branches?.length)
const isGate = computed(() => !!props.data.gateActions?.length)
const isAppPreviewReview = computed(() => !!props.data.appPreviewReview)
const isStructuredGate = computed(() => !!props.data.structuredExits?.length)
const def = computed(() => NODE_DEFS.value[props.data.type])
const displayLabel = computed(() => {
  void locale.value
  return resolveNodeDisplayLabel(props.data.label, props.data.type, t, {
    nodeId: props.id,
    typeLabel: def.value?.label,
  })
})
const hex = computed(() => nodeColorHex(props.data.type))
const status = computed(() => props.data.status)

const ring = computed(() => {
  switch (status.value) {
    case 'running':
      return 'border-info shadow-[0_0_0_1px_rgba(77,163,255,0.6),0_0_20px_-4px_rgba(77,163,255,0.5)]'
    case 'completed':
      return 'border-ok/60'
    case 'failed':
      return 'border-err/70'
    case 'waiting_human':
      return 'border-warn shadow-[0_0_0_1px_rgba(251,191,36,0.5)]'
    default:
      return props.selected ? 'border-accent shadow-glow' : 'border-line hover:border-line-strong'
  }
})

const statusBadge = computed(() => {
  switch (status.value) {
    case 'running':
      return { icon: 'dot', cls: 'bg-info text-white animate-pulseglow' }
    case 'completed':
      return { icon: 'check', cls: 'bg-ok text-base' }
    case 'failed':
      return { icon: 'alert', cls: 'bg-err text-white' }
    case 'waiting_human':
      return { icon: 'gate', cls: 'bg-warn text-base' }
    default:
      return null
  }
})

const subtitle = computed(() => {
  if (isAppPreviewReview.value) return t('pages.workflowEditor.canvas.appPreviewSubtitle')
  if (props.data.type === 'human_gate') return t('pages.workflowEditor.canvas.humanGateSubtitle')
  return def.value?.label || props.data.type
})
</script>

<template>
  <div
    class="group relative w-[176px] rounded-lg border bg-elevated/95 backdrop-blur transition"
    :class="ring"
  >
    <Handle type="target" :position="Position.Left" />
    <div class="absolute left-0 top-0 h-full w-1 rounded-l-lg" :style="{ background: hex }" />

    <div class="flex items-center gap-2.5 px-3 py-2.5 pl-4">
      <div
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md"
        :style="{ background: hex + '22', color: hex }"
      >
        <Icon :name="def?.icon || 'agent'" :size="17" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="truncate text-[13px] font-semibold text-txt">{{ displayLabel }}</div>
        <div class="truncate text-[11px] text-txt3">
          {{ subtitle }}
        </div>
      </div>
      <div
        v-if="data.checkpoint"
        class="flex h-5 shrink-0 items-center gap-0.5 rounded-full border border-warn/40 bg-warn/10 px-1.5 text-[9px] font-medium text-warn"
        :title="t('pages.workflowEditor.canvas.checkpointTitle')"
      >
        <Icon name="flag" :size="9" />{{ t('pages.workflowEditor.canvas.checkpoint') }}
      </div>
    </div>

    <div
      v-if="isAppPreviewReview"
      class="border-t border-line/60 px-3 py-1.5 pl-4 text-[11px] text-txt3"
      data-testid="app-preview-body"
    >
      {{ t('pages.workflowEditor.canvas.appPreviewBody') }}
    </div>

    <div v-if="isBranch" class="border-t border-line/60">
      <div
        v-for="b in data.branches"
        :key="b.id"
        class="relative flex items-center gap-1.5 px-3 py-1.5 pl-4 text-[11px]"
      >
        <span
          class="rounded px-1 py-0.5 font-mono text-[9px] font-semibold"
          :class="b.isDefault ? 'bg-warn/15 text-warn' : 'bg-accent-dim text-accent-2'"
        >{{ b.isDefault ? 'ELSE' : 'IF' }}</span>
        <span class="min-w-0 flex-1 truncate text-txt2" :title="b.label">{{ b.label }}</span>
        <Handle :id="b.id" type="source" :position="Position.Right" />
      </div>
    </div>

    <div v-if="isGate" class="border-t border-line/60">
      <div
        v-for="a in data.gateActions"
        :key="a.id"
        class="relative flex items-center gap-1.5 px-3 py-1.5 pl-4 text-[11px]"
      >
        <span class="rounded bg-warn/15 px-1 py-0.5 font-mono text-[9px] font-semibold text-warn">{{ t('common.action') }}</span>
        <span class="min-w-0 flex-1 truncate text-txt2" :title="a.label">{{ a.label || a.id }}</span>
        <Handle :id="`action-${a.id}`" type="source" :position="Position.Right" />
      </div>
      <div class="px-3 pb-1.5 pl-4 text-[9px] text-txt3">{{ t('pages.workflowEditor.canvas.gateActionFallback') }}</div>
    </div>

    <div v-if="isStructuredGate" class="border-t border-line/60">
      <div
        v-for="a in data.structuredExits"
        :key="a.id"
        class="relative flex items-center gap-1.5 px-3 py-1.5 pl-4 text-[11px]"
      >
        <span
          class="rounded px-1 py-0.5 font-mono text-[9px] font-semibold"
          :class="a.tone === 'ok' ? 'bg-ok/15 text-ok' : 'bg-err/15 text-err'"
        >{{ a.label }}</span>
        <span class="min-w-0 flex-1" />
        <Handle :id="`action-${a.id}`" type="source" :position="Position.Right" />
      </div>
      <div class="px-3 pb-1.5 pl-4 text-[9px] text-txt3">{{ t('pages.workflowEditor.canvas.structuredExitFallback') }}</div>
    </div>

    <div
      v-if="isAppPreviewReview"
      class="absolute -right-2 -top-2 z-10 border border-base bg-ok px-1.5 py-0.5 text-[10px] font-bold text-base"
    >
      {{ t('pages.workflowEditor.canvas.reviewBadge') }}
    </div>
    <div
      v-else-if="statusBadge"
      class="absolute -right-2 -top-2 flex h-5 w-5 items-center justify-center rounded-full border-2 border-base"
      :class="statusBadge.cls"
    >
      <Icon :name="statusBadge.icon" :size="11" />
    </div>

    <Handle v-if="isGate || isStructuredGate" type="source" :position="Position.Bottom" />
    <Handle v-else-if="!isBranch" type="source" :position="Position.Right" />
  </div>
</template>

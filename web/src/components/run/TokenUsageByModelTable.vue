<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  effectiveModelUsageRows,
  fmtTokenCount,
  mergeTokenUsageByModel,
  shouldShowUnknownVisual,
  TOKEN_USAGE_SOURCE_BRIDGE,
  tokenUsageTotal,
  unknownDisplayName,
} from '@/lib/run/tokenUsage'
import type { TokenUsage, TokenUsageByModel } from '@/lib/shared/types'
import UnknownModelBadge from '@/components/ui/UnknownModelBadge.vue'

const props = defineProps<{
  /** Merged run total (optional; derived from by-model when omitted). */
  usage?: TokenUsage | null
  usageByModel?: TokenUsageByModel | null
  /** Merge multiple process-level by-model maps (StateRun rows). */
  parts?: Array<{ usage?: TokenUsage | null; usageByModel?: TokenUsageByModel | null }>
  /** Project-level alias for the unknown token bucket. */
  unknownModelDisplayName?: string | null
}>()

const { t } = useI18n()

const mergedByModel = computed(() => {
  if (props.parts?.length) {
    return mergeTokenUsageByModel(...props.parts.map((p) => p.usageByModel))
  }
  return props.usageByModel ?? null
})

const mergedUsage = computed(() => {
  if (props.usage != null) return props.usage
  if (props.parts?.length) {
    let has = false
    const u: TokenUsage = { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 }
    for (const p of props.parts) {
      if (p.usage == null) continue
      has = true
      u.inputTokens += p.usage.inputTokens || 0
      u.outputTokens += p.usage.outputTokens || 0
      u.cacheReadTokens += p.usage.cacheReadTokens || 0
      u.cacheWriteTokens += p.usage.cacheWriteTokens || 0
    }
    return has ? u : null
  }
  return null
})

const rows = computed(() => effectiveModelUsageRows(mergedUsage.value, mergedByModel.value))
const total = computed(() => (mergedUsage.value != null ? tokenUsageTotal(mergedUsage.value) : null))
const rowsSum = computed(() => rows.value.reduce((s, r) => s + r.total, 0))

function sourceLabel(source: string, filled: boolean): string {
  if (filled || source === TOKEN_USAGE_SOURCE_BRIDGE) return t('pages.tokenByModel.sourceBridge')
  if (source === 'unknown') return t('pages.tokenByModel.sourceUnknown')
  return t('pages.tokenByModel.sourceUpstream')
}

function modelLabel(modelKey: string, unknown: boolean): string {
  if (!unknown) return modelKey
  return unknownDisplayName(modelKey, props.unknownModelDisplayName)
}

function showUnknownVisual(modelKey: string, unknown: boolean): boolean {
  return shouldShowUnknownVisual(unknown, modelLabel(modelKey, unknown))
}
</script>

<template>
  <div v-if="total != null" data-testid="run-token-by-model" class="mt-3 border border-line bg-surface">
    <div class="flex items-baseline justify-between gap-2 border-b border-line px-3 py-2">
      <h4 class="m-0 text-[13px] font-semibold text-txt">{{ t('pages.tokenByModel.modelTableTitle') }}</h4>
      <span class="font-mono text-[11px] tabular-nums text-txt3">
        Σ {{ fmtTokenCount(rowsSum) }}
        <span v-if="total != null && rowsSum === total" class="ml-1 text-ok">= {{ t('pages.executionStats.totalTokens') }}</span>
      </span>
    </div>
    <div class="overflow-x-auto">
      <table class="w-full min-w-[520px] border-collapse text-left text-xs">
        <thead>
          <tr class="border-b border-line text-txt3">
            <th class="px-3 py-2 font-medium">{{ t('pages.tokenByModel.colModel') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('pages.tokenByModel.colSource') }}</th>
            <th class="px-3 py-2 font-medium">input</th>
            <th class="px-3 py-2 font-medium">output</th>
            <th class="px-3 py-2 font-medium">cacheRead</th>
            <th class="px-3 py-2 font-medium">cacheWrite</th>
            <th class="px-3 py-2 font-medium">{{ t('pages.tokenByModel.colSubtotal') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.modelKey"
            class="border-b border-line/70"
            :data-model="row.modelKey"
            :data-filled="row.filled ? '1' : '0'"
            :data-unknown="row.unknown ? '1' : '0'"
          >
            <td
              class="px-3 py-2 font-medium"
              :class="showUnknownVisual(row.modelKey, row.unknown) ? 'text-txt3' : row.filled ? 'text-ok' : 'text-txt'"
            >
              <span class="inline-flex max-w-full items-center gap-1.5">
                <span class="min-w-0 truncate">{{ modelLabel(row.modelKey, row.unknown) }}</span>
                <UnknownModelBadge v-if="showUnknownVisual(row.modelKey, row.unknown)" />
              </span>
            </td>
            <td class="px-3 py-2 text-txt3">{{ sourceLabel(row.source, row.filled) }}</td>
            <td class="px-3 py-2 font-mono tabular-nums">{{ fmtTokenCount(row.inputTokens) }}</td>
            <td class="px-3 py-2 font-mono tabular-nums">{{ fmtTokenCount(row.outputTokens) }}</td>
            <td class="px-3 py-2 font-mono tabular-nums">{{ fmtTokenCount(row.cacheReadTokens) }}</td>
            <td class="px-3 py-2 font-mono tabular-nums">{{ fmtTokenCount(row.cacheWriteTokens) }}</td>
            <td class="px-3 py-2 font-mono font-semibold tabular-nums text-txt">{{ fmtTokenCount(row.total) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

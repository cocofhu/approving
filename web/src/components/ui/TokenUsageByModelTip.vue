<script setup lang="ts">
/**
 * Click-to-expand token tip with per-model blocks (Demo-aligned).
 * Does not rely on hover — works on touch. Close via button or toggle chip.
 */
import { computed, ref, watch, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  effectiveModelUsageRows,
  fmtCompactTokenCount,
  fmtTokenCount,
  TOKEN_USAGE_SOURCE_BRIDGE,
  tokenUsageTotal,
  unknownDisplayName,
} from '@/lib/run/tokenUsage'
import type { TokenUsage, TokenUsageByModel } from '@/lib/shared/types'

/**
 * `open` uses runtime Boolean + default undefined so omitted prop is uncontrolled.
 * Type-only `open?: boolean` is cast to Boolean and defaults to false, locking
 * the tip closed when parents (e.g. PmLeaderChat) do not bind v-model:open.
 */
const props = defineProps({
  usage: { type: Object as PropType<TokenUsage | null>, default: null },
  usageByModel: { type: Object as PropType<TokenUsageByModel | null>, default: null },
  open: { type: Boolean, default: undefined },
  unknownModelDisplayName: { type: String as PropType<string | null>, default: null },
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const internalOpen = ref(false)

/** Uncontrolled when parent omits `open` (undefined sentinel). */
const isControlled = computed(() => props.open !== undefined)

const isOpen = computed({
  get: () => (isControlled.value ? !!props.open : internalOpen.value),
  set: (v: boolean) => {
    if (!isControlled.value) internalOpen.value = v
    emit('update:open', v)
  },
})

watch(
  () => [props.usage, props.usageByModel] as const,
  () => {
    // Keep tip closed when the underlying usage identity changes.
  },
)

const total = computed(() => (props.usage != null ? tokenUsageTotal(props.usage) : null))
const rows = computed(() => effectiveModelUsageRows(props.usage, props.usageByModel))

function toggle() {
  isOpen.value = !isOpen.value
}

function close() {
  isOpen.value = false
}

function sourceLabel(source: string, filled: boolean): string {
  if (filled || source === TOKEN_USAGE_SOURCE_BRIDGE) {
    return t('pages.tokenByModel.sourceBridge')
  }
  if (source === 'unknown') return t('pages.tokenByModel.sourceUnknown')
  return t('pages.tokenByModel.sourceUpstream')
}

function modelLabel(modelKey: string, unknown: boolean): string {
  if (!unknown) return modelKey
  return unknownDisplayName(modelKey, props.unknownModelDisplayName)
}

defineExpose({ toggle, close, isOpen })
</script>

<template>
  <div v-if="total != null" class="relative inline-flex" data-testid="token-by-model-tip-host">
    <button
      type="button"
      class="usage-trigger inline-flex items-center gap-1 border border-line bg-elevated px-2 py-0.5 font-mono text-xs tabular-nums text-txt2 hover:border-accent/40 hover:text-txt"
      data-testid="token-by-model-trigger"
      :aria-expanded="isOpen"
      @click.stop="toggle"
    >
      Tokens · {{ fmtCompactTokenCount(total) }}
    </button>
    <div
      v-if="isOpen"
      role="dialog"
      :aria-label="t('pages.tokenByModel.tipTitle')"
      data-testid="token-by-model-tip"
      class="absolute right-0 top-[calc(100%+8px)] z-30 w-[min(320px,calc(100vw-24px))] border border-line bg-[#111827] px-3 py-2.5 text-left text-[#f9fafb] shadow-lg"
    >
      <div class="mb-2 flex items-center justify-between gap-2">
        <span class="text-[11px] font-semibold tracking-wide text-[#9ca3af]">
          {{ t('pages.tokenByModel.tipTitle') }}
        </span>
        <button
          type="button"
          class="border border-white/10 px-2 py-0.5 text-[11px] text-[#9ca3af] hover:text-white"
          data-testid="token-by-model-close"
          @click.stop="close"
        >
          {{ t('pages.tokenByModel.close') }}
        </button>
      </div>
      <div class="mb-2 font-mono text-lg font-bold tabular-nums">
        {{ fmtTokenCount(total) }}
        <span class="ml-1.5 text-[11px] font-medium text-[#9ca3af]">tokens</span>
      </div>
      <div class="grid gap-2" data-testid="token-by-model-blocks">
        <div
          v-for="row in rows"
          :key="row.modelKey"
          class="border border-white/10 bg-white/5 px-2 py-1.5"
          :data-model="row.modelKey"
          :data-filled="row.filled ? '1' : '0'"
        >
          <div class="mb-1 flex items-center justify-between gap-2 text-xs">
            <strong
              class="min-w-0 truncate font-semibold"
              :class="row.unknown ? 'text-[#a1a1aa]' : row.filled ? 'text-[#6ee7b7]' : 'text-[#f9fafb]'"
            >{{ modelLabel(row.modelKey, row.unknown) }}</strong>
            <span class="shrink-0 font-mono tabular-nums text-[#c7cbd4]">{{ fmtTokenCount(row.total) }}</span>
          </div>
          <div class="mb-1 text-[10px] text-[#9ca3af]">{{ sourceLabel(row.source, row.filled) }}</div>
          <div class="grid grid-cols-2 gap-1 text-[11px] text-[#d1d5db]">
            <span>in {{ fmtTokenCount(row.inputTokens) }}</span>
            <span>out {{ fmtTokenCount(row.outputTokens) }}</span>
            <span>cR {{ fmtTokenCount(row.cacheReadTokens) }}</span>
            <span>cW {{ fmtTokenCount(row.cacheWriteTokens) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

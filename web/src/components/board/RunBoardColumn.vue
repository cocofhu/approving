<script setup lang="ts">
import RunBoardCard from './RunBoardCard.vue'
import type { Run } from '@/lib/shared/types'

const props = withDefaults(
  defineProps<{
    title: string
    hint?: string
    accent?: 'active' | 'running' | 'waiting' | 'done' | 'extra'
    items: Run[]
    emptyText: string
    truncatedHint?: string
    /** Server-reported total; used for badge when truncated. */
    total?: number
    loading?: boolean
    loadingText?: string
    /** Fill parent height without max-height cap (dashboard preview only). */
    fill?: boolean
    /**
     * When true, column header is a focusable control that emits activate-header
     * (project full board only; Dashboard leaves this off).
     */
    headerActivatable?: boolean
    /** Status key emitted with activate-header when headerActivatable. */
    status?: string
  }>(),
  { accent: 'extra', total: 0, loading: false, fill: false, headerActivatable: false },
)

const emit = defineEmits<{
  (e: 'select', run: Run): void
  (e: 'activate-header', status: string): void
}>()

const accentClass: Record<string, string> = {
  active: 'border-t-2 border-t-accent-2',
  running: 'border-t-2 border-t-info',
  waiting: 'border-t-2 border-t-warn',
  done: 'border-t-2 border-t-ok',
  extra: 'border-t-2 border-t-txt3',
}

function badgeLabel(): string {
  const n = props.items.length
  const total = props.total || 0
  if (total > n && n > 0) return `${n}/${total}`
  if (total > n) return String(total)
  return String(n)
}

function onActivateHeader() {
  if (!props.headerActivatable) return
  emit('activate-header', props.status || '')
}
</script>

<template>
  <div
    class="flex min-h-[200px] flex-col border border-line bg-base"
    :class="[accentClass[accent] || accentClass.extra, fill ? 'h-full' : '']"
    data-testid="run-board-column"
  >
    <component
      :is="headerActivatable ? 'button' : 'div'"
      :type="headerActivatable ? 'button' : undefined"
      class="flex w-full items-center gap-2 border-b border-line bg-surface px-3 py-2.5 text-left"
      :class="
        headerActivatable
          ? 'cursor-pointer transition hover:bg-elevated focus-visible:bg-elevated focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-[-1px] focus-visible:outline-accent'
          : ''
      "
      :aria-haspopup="headerActivatable ? 'dialog' : undefined"
      :data-testid="headerActivatable ? 'run-board-column-header' : undefined"
      @click="headerActivatable ? onActivateHeader() : undefined"
    >
      <span class="text-[13px] font-semibold text-txt">{{ title }}</span>
      <span v-if="hint" class="text-[11px] text-txt3">{{ hint }}</span>
      <span
        class="ml-auto inline-flex h-5 min-w-[22px] items-center justify-center border border-line bg-elevated px-1.5 text-[11px] font-semibold text-txt2"
        data-testid="run-board-column-count"
      >
        {{ badgeLabel() }}
      </span>
    </component>
    <div
      class="scroll-area flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-2.5"
      :style="fill ? undefined : { maxHeight: 'min(52vh, 420px)' }"
      data-testid="run-board-column-body"
    >
      <template v-if="loading && !items.length">
        <div
          class="border border-dashed border-line px-3 py-7 text-center text-xs text-txt3"
          data-testid="run-board-column-loading"
        >
          {{ loadingText || '…' }}
        </div>
      </template>
      <template v-else-if="items.length">
        <RunBoardCard v-for="r in items" :key="r.id" :run="r" @select="emit('select', $event)" />
      </template>
      <div
        v-else
        class="border border-dashed border-line px-3 py-7 text-center text-xs text-txt3"
        data-testid="run-board-column-empty"
      >
        {{ emptyText }}
      </div>
      <p v-if="truncatedHint" class="px-1 text-[11px] text-txt3">{{ truncatedHint }}</p>
    </div>
    <div v-if="$slots.footer" class="px-3 pb-3 pt-1">
      <slot name="footer" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TokenStatsWorkflow } from '@/lib/types'
import { fmtTokenCount } from '@/lib/tokenUsage'

const props = defineProps<{
  workflows: TokenStatsWorkflow[]
}>()

const { t } = useI18n()
const activeIdx = ref<number | null>(null)

const maxTotal = computed(() => Math.max(1, ...props.workflows.map((w) => w.total || 0)))

function displayName(w: TokenStatsWorkflow): string {
  if (w.other) return t('pages.board.tokenStats.other')
  return w.name || w.workflowId || '—'
}

function barWidth(total: number): string {
  return `${Math.max(2, Math.round((total / maxTotal.value) * 100))}%`
}

const tip = ref({ show: false, x: 0, y: 0, idx: -1 })

function showTip(i: number, ev: MouseEvent) {
  activeIdx.value = i
  const wrap = (ev.currentTarget as HTMLElement).closest('[data-testid="token-rank-list"]') as HTMLElement | null
  if (!wrap) return
  const rect = wrap.getBoundingClientRect()
  tip.value = {
    show: true,
    x: ev.clientX - rect.left,
    y: ev.clientY - rect.top,
    idx: i,
  }
}

function hideTip() {
  activeIdx.value = null
  tip.value = { ...tip.value, show: false }
}

const tipRow = computed(() => (tip.value.idx >= 0 ? props.workflows[tip.value.idx] : null))
</script>

<template>
  <ul data-testid="token-rank-list" class="token-rank relative m-0 grid list-none gap-2 p-0">
    <li
      v-for="(w, i) in workflows"
      :key="(w.workflowId || w.name) + '-' + i"
      class="grid cursor-pointer grid-cols-[22px_1fr_auto] items-center gap-2 rounded-md px-0.5 py-1"
      :class="[
        w.other ? 'token-rank-other' : '',
        activeIdx === i ? 'bg-elevated' : 'hover:bg-elevated/80',
      ]"
      @mouseenter="showTip(i, $event)"
      @mousemove="showTip(i, $event)"
      @mouseleave="hideTip"
      @click="showTip(i, $event)"
    >
      <span
        class="grid h-[22px] w-[22px] place-items-center rounded-md text-[11px] font-bold"
        :class="i < 3 && !w.other ? 'bg-accent-dim text-accent-2' : 'bg-elevated text-txt3'"
      >
        {{ w.other ? '·' : i + 1 }}
      </span>
      <div class="min-w-0">
        <div class="truncate text-xs font-medium text-txt">{{ displayName(w) }}</div>
        <div class="mt-1 h-2 overflow-hidden rounded-full bg-[#f1f3f6]">
          <div
            class="h-full rounded-full transition-[width] duration-300"
            :class="w.other ? 'bg-gradient-to-r from-slate-400 to-slate-300' : 'bg-gradient-to-r from-[#6d5cff] to-[#9b8cff]'"
            :style="{ width: barWidth(w.total) }"
          />
        </div>
      </div>
      <span class="whitespace-nowrap text-xs tabular-nums text-txt3">{{ fmtTokenCount(w.total) }}</span>
    </li>
    <div
      v-if="tip.show && tipRow"
      data-testid="token-rank-tooltip"
      class="pointer-events-none absolute z-10 min-w-[120px] rounded-lg bg-[#1a1d23] px-2.5 py-2 text-[11px] text-white shadow-lg"
      :style="{ left: tip.x + 'px', top: tip.y + 'px', transform: 'translate(-10%, -110%)' }"
    >
      <div class="mb-1 font-semibold">{{ displayName(tipRow) }}</div>
      <div class="flex justify-between gap-3 text-[#c7cbd4]">
        <span>{{ t('pages.board.tokenStats.value') }}</span>
        <b class="font-normal tabular-nums text-white">{{ fmtTokenCount(tipRow.total) }}</b>
      </div>
    </div>
  </ul>
</template>

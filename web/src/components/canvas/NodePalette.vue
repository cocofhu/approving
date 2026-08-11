<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import { nodeColorHex } from '@/data/nodeRegistry'
import { useNodeDefs, usePaletteGroups } from '@/lib/run/useNodeDefs'
import type { NodeType } from '@/lib/shared/types'

const { t } = useI18n()
const { NODE_DEFS } = useNodeDefs()
const { PALETTE_GROUPS } = usePaletteGroups()

const q = ref('')

const groups = computed(() =>
  PALETTE_GROUPS.value.map((g) => ({
    title: g.title,
    items: g.types
      .map((type) => NODE_DEFS.value[type])
      .filter((d) => !q.value || d.label.includes(q.value) || d.type.includes(q.value)),
  })).filter((g) => g.items.length),
)

function onDragStart(ev: DragEvent, type: NodeType) {
  ev.dataTransfer?.setData('application/approving-node', type)
  if (ev.dataTransfer) ev.dataTransfer.effectAllowed = 'move'
}
</script>

<template>
  <div class="flex h-full w-[248px] shrink-0 flex-col border-r border-line bg-surface">
    <div class="border-b border-line p-3">
      <div class="relative">
        <Icon name="search" :size="15" class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-txt3" />
        <input v-model="q" class="input pl-8" :placeholder="t('pages.workflowEditor.palette.searchPlaceholder')" />
      </div>
    </div>
    <div class="scroll-area flex-1 overflow-y-auto p-3">
      <div v-for="g in groups" :key="g.title" class="mb-4">
        <div class="px-1 pb-2 text-[10px] font-semibold uppercase tracking-wider text-txt3">{{ g.title }}</div>
        <div
          v-for="d in g.items"
          :key="d.type"
          class="group mb-2 flex cursor-grab items-center gap-2.5 rounded-lg border border-line bg-elevated px-2.5 py-2 transition hover:border-line-strong hover:bg-overlay active:cursor-grabbing"
          draggable="true"
          @dragstart="onDragStart($event, d.type)"
        >
          <div
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md"
            :style="{ background: nodeColorHex(d.type) + '22', color: nodeColorHex(d.type) }"
          >
            <Icon :name="d.icon" :size="16" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="truncate text-[13px] font-medium text-txt">{{ d.label }}</div>
            <div class="truncate text-[11px] text-txt3">{{ d.desc }}</div>
          </div>
          <Icon name="more" :size="14" class="text-txt3 opacity-0 transition group-hover:opacity-100" />
        </div>
      </div>
    </div>
    <div class="border-t border-line p-3 text-[11px] leading-relaxed text-txt3">
      {{ t('pages.workflowEditor.palette.footerHint') }}
    </div>
  </div>
</template>

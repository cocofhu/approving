<script setup lang="ts">
import { toRef } from 'vue'
import type { ReactAnnotation } from '@/lib/types'
import { useReviewTextSelection } from '@/lib/useReviewTextSelection'
import { useToast } from '@/lib/useToast'

const props = defineProps<{
  enabled: boolean
  root: HTMLElement | null | undefined
}>()

const emit = defineEmits<{
  add: [ann: ReactAnnotation]
}>()

const toast = useToast()

const { visible, style, clearSelection, takeSelection, preserveOnMouseDown } = useReviewTextSelection({
  enabled: toRef(props, 'enabled'),
  root: toRef(props, 'root'),
  onCrossField: () => toast.warn('请在同一字段内划选'),
})

function onAdd() {
  const sel = takeSelection()
  if (!sel?.quote) {
    toast.warn('没有可用选区')
    return
  }
  emit('add', {
    quote: sel.quote,
    jsonPath: sel.jsonPath,
    label: sel.label || sel.jsonPath,
  })
  clearSelection()
}
</script>

<template>
  <Teleport to="body">
    <div
      v-show="enabled && visible"
      data-selection-add-to-chat
      data-testid="selection-add-to-chat"
      class="fixed z-50 inline-flex items-center border border-accent-2/55 bg-elevated shadow-lg"
      :style="style"
      role="toolbar"
      aria-label="选区操作"
    >
      <button
        type="button"
        class="whitespace-nowrap px-2.5 py-1.5 text-[12px] font-medium text-accent-2 hover:bg-accent/18"
        data-testid="selection-add-to-chat-btn"
        @mousedown="preserveOnMouseDown"
        @click="onAdd"
      >
        添加到聊天 <span class="text-[11px] font-normal text-txt3">/ Add to Chat</span>
      </button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { useReviewAnnotate } from '@/lib/inbox/reviewAnnotate'
import Icon from '../../ui/Icon.vue'

// A hover-revealed "⤴ 标注" affordance for one leaf field of a structured
// product. Renders nothing outside a review panel (inject returns null / not
// enabled), so read-only product views are unaffected.
const props = defineProps<{ jsonPath: string; label?: string }>()

const channel = useReviewAnnotate()
function pick(e: MouseEvent) {
  e.stopPropagation()
  channel?.annotate({ jsonPath: props.jsonPath, label: props.label || props.jsonPath })
}
</script>

<template>
  <button
    v-if="channel && channel.enabled"
    type="button"
    class="ml-1 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded border border-accent/40 text-accent-2 opacity-0 transition hover:bg-accent/15 group-hover:opacity-100"
    :title="`标注 ${jsonPath}`"
    @click="pick"
  >
    <Icon name="crosshair" :size="10" />
  </button>
</template>

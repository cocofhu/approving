<script setup lang="ts">
import { ref } from 'vue'
import Icon from './Icon.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title?: string
    width?: number
    hideBody?: boolean
    bodyOverflow?: 'auto' | 'hidden'
    bodyMinHeight?: number
    /**
     * When false, clicking the backdrop does not emit close (default true).
     * Esc-to-close is intentionally out of scope (no keydown listener). If closeOnEsc is
     * added later, default it to false / opt-in so closeOnBackdrop=false callers stay safe.
     */
    closeOnBackdrop?: boolean
  }>(),
  { bodyOverflow: 'auto', closeOnBackdrop: true },
)
const emit = defineEmits<{ (e: 'close'): void }>()

function onBackdropClick() {
  if (props.closeOnBackdrop) emit('close')
}

const scrollAreaEl = ref<HTMLElement | null>(null)

defineExpose({ scrollAreaEl })
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/60 backdrop-blur-sm" @click="onBackdropClick" />
        <div
          class="relative flex max-h-[88vh] w-full flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-card"
          :style="{ maxWidth: (width || 560) + 'px' }"
        >
          <div class="flex h-14 shrink-0 items-center gap-2 border-b border-line px-5">
            <div v-if="$slots.header" class="flex min-w-0 flex-1 items-center">
              <slot name="header" />
            </div>
            <div v-else class="flex-1 text-[15px] font-semibold text-txt">{{ title }}</div>
            <button class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-txt3 hover:bg-elevated hover:text-txt" @click="emit('close')">
              <Icon name="close" :size="18" />
            </button>
          </div>
          <div
            v-if="!hideBody"
            ref="scrollAreaEl"
            class="scroll-area modal-scroll-area min-h-0 flex-1 p-5"
            :class="bodyOverflow === 'hidden' ? 'overflow-hidden' : 'overflow-y-auto'"
            :style="bodyMinHeight ? { minHeight: `${bodyMinHeight}px` } : undefined"
          >
            <slot />
          </div>
          <div v-if="$slots.footer" class="flex shrink-0 items-center justify-end gap-2 border-t border-line p-4">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}
.modal-enter-active > div:last-child,
.modal-leave-active > div:last-child {
  transition: transform 0.22s cubic-bezier(0.16, 1, 0.3, 1);
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
.modal-enter-from > div:last-child,
.modal-leave-to > div:last-child {
  transform: translateY(12px) scale(0.98);
}
</style>

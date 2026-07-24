<script setup lang="ts">
import Icon from './Icon.vue'

defineProps<{ open: boolean; title?: string; width?: number }>()
const emit = defineEmits<{ (e: 'close'): void }>()
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="open" class="fixed inset-0 z-40">
        <div class="absolute inset-0 bg-black/50" @click="emit('close')" />
        <div
          class="absolute right-0 top-0 flex h-full flex-col border-l border-line bg-surface shadow-drawer"
          :style="{ width: (width || 420) + 'px', maxWidth: '100vw' }"
        >
          <div class="flex h-14 shrink-0 items-center gap-2 border-b border-line px-4">
            <div class="flex-1 text-sm font-semibold text-txt">{{ title }}</div>
            <button class="flex h-8 w-8 items-center justify-center rounded-md text-txt3 hover:bg-elevated hover:text-txt" @click="emit('close')">
              <Icon name="close" :size="18" />
            </button>
          </div>
          <div class="scroll-area min-h-0 flex-1 overflow-y-auto">
            <slot />
          </div>
          <div v-if="$slots.footer" class="shrink-0 border-t border-line p-4">
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-enter-active > div:last-child,
.drawer-leave-active > div:last-child {
  transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from > div:last-child,
.drawer-leave-to > div:last-child {
  transform: translateX(100%);
}
</style>

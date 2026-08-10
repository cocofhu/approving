<script setup lang="ts">
import { useToast } from '@/lib/useToast'

const { toasts } = useToast()
</script>

<template>
  <Teleport to="body">
    <div
      class="pointer-events-none fixed bottom-6 right-6 z-[100] flex flex-col gap-2"
      role="status"
      aria-live="polite"
      data-testid="toast-host"
    >
      <TransitionGroup name="toast">
        <div
          v-for="t in toasts"
          :key="t.id"
          class="border border-line bg-elevated px-4 py-2.5 text-[13px] font-medium text-txt shadow-card"
          :class="{
            'border-ok/40 text-ok': t.type === 'success',
            'border-err/40 text-err': t.type === 'error',
            'border-warn/40 text-warn': t.type === 'warn',
          }"
        >
          {{ t.message }}
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>

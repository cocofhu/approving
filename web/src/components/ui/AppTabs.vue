<script setup lang="ts">
defineProps<{
  tabs: {
    id: string
    label: string
    /** Visually muted + line-through; click does not change model (optional disabledClick). */
    ghosted?: boolean
    disabled?: boolean
  }[]
  modelValue: string
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'disabled-click', id: string): void
}>()

function onTabClick(t: { id: string; ghosted?: boolean; disabled?: boolean }) {
  if (t.ghosted || t.disabled) {
    emit('disabled-click', t.id)
    return
  }
  emit('update:modelValue', t.id)
}
</script>

<template>
  <div class="scroll-area -mx-1 overflow-x-auto">
    <div class="flex min-w-max items-center gap-1 border-b border-line px-1">
      <button
        v-for="t in tabs"
        :key="t.id"
        type="button"
        class="relative -mb-px shrink-0 px-3.5 py-2.5 text-sm font-medium transition"
        :class="[
          t.ghosted || t.disabled
            ? 'cursor-not-allowed text-txt3/40 line-through'
            : modelValue === t.id
              ? 'text-txt'
              : 'text-txt3 hover:text-txt2',
        ]"
        :aria-disabled="t.ghosted || t.disabled ? 'true' : undefined"
        @click="onTabClick(t)"
      >
        {{ t.label }}
        <span
          v-if="modelValue === t.id && !t.ghosted && !t.disabled"
          class="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-accent"
        />
      </button>
    </div>
  </div>
</template>

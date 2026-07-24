<script setup lang="ts">
defineProps<{ tabs: { id: string; label: string }[]; modelValue: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
</script>

<template>
  <div class="scroll-area -mx-1 overflow-x-auto">
    <div class="flex min-w-max items-center gap-1 border-b border-line px-1">
      <button
        v-for="t in tabs"
        :key="t.id"
        class="relative -mb-px shrink-0 px-3.5 py-2.5 text-sm font-medium transition"
        :class="modelValue === t.id ? 'text-txt' : 'text-txt3 hover:text-txt2'"
        @click="emit('update:modelValue', t.id)"
      >
        {{ t.label }}
        <span
          v-if="modelValue === t.id"
          class="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-accent"
        />
      </button>
    </div>
  </div>
</template>

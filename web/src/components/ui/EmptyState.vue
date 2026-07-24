<script setup lang="ts">
import Icon from './Icon.vue'
defineProps<{ icon?: string; title: string; desc?: string }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center px-6 py-14 text-center">
    <div class="mb-3 flex h-12 w-12 items-center justify-center rounded-xl border border-line bg-elevated text-txt3">
      <Icon :name="icon || 'doc'" :size="22" />
    </div>
    <div class="text-sm font-medium text-txt">{{ title }}</div>
    <div v-if="desc" class="mt-1 max-w-sm text-xs text-txt3">{{ desc }}</div>
    <div class="mt-4"><slot /></div>
  </div>
</template>

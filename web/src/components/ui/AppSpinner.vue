<script setup lang="ts">
import Icon from './Icon.vue'

withDefaults(
  defineProps<{
    size?: number | string
  }>(),
  { size: 14 },
)
</script>

<template>
  <Icon name="spinner" :size="size" class="app-spinner" aria-hidden="true" />
</template>

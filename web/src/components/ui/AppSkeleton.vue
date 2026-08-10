<script setup lang="ts">
/** Generic shell skeleton: title + 2 cards + 5 rows. Not route-density specific. */
</script>

<template>
  <div class="app-skeleton" aria-hidden="true" data-testid="app-skeleton">
    <div class="app-skeleton__block app-skeleton__title" />
    <div class="mb-2.5 grid grid-cols-2 gap-2.5">
      <div class="app-skeleton__block app-skeleton__card" />
      <div class="app-skeleton__block app-skeleton__card" />
    </div>
    <div v-for="i in 5" :key="i" class="app-skeleton__block app-skeleton__row" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { serviceCommit } from '@/lib/composables/useServiceCommit'

const route = useRoute()

const sha = computed(() => {
  if (route.name !== 'dashboard') return ''
  return serviceCommit.value
})
</script>

<template>
  <span
    v-if="sha"
    data-testid="service-commit-badge"
    class="pointer-events-none absolute right-[14px] bottom-[calc(10px+env(safe-area-inset-bottom,0px))] z-10 font-mono text-[11px] text-txt3"
    aria-hidden="true"
  >{{ sha }}</span>
</template>

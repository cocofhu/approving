<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppShell from './components/shell/AppShell.vue'
import ToastHost from './components/ui/ToastHost.vue'
import { locale, updateDocumentTitle } from '@/lib/locale'

const route = useRoute()
const bareLayout = computed(() => !!route.meta.bare)

watch(locale, () => {
  updateDocumentTitle(route.meta.titleKey as string | undefined)
})
</script>

<template>
  <AppShell v-if="!bareLayout">
    <router-view />
  </AppShell>
  <router-view v-else />
  <ToastHost />
</template>

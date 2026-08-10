<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppShell from './components/shell/AppShell.vue'
import ToastHost from './components/ui/ToastHost.vue'
import AppSkeleton from './components/ui/AppSkeleton.vue'
import { locale, updateDocumentTitle } from '@/lib/locale'
import { useAuth } from '@/lib/useAuth'
import { useDelayedBusy } from '@/lib/useDelayedBusy'
import { useRoutePending } from '@/lib/routePending'
import { useLoadingAnnouncer } from '@/lib/loadingAnnouncer'

const route = useRoute()
const bareLayout = computed(() => !!route.meta.bare)
const auth = useAuth()
const routePending = useRoutePending()
const { liveMessage } = useLoadingAnnouncer()

const authBlocking = computed(() => !bareLayout.value && (!auth.ready.value || !auth.user.value))
const authDelayed = useDelayedBusy({ mode: 'initial' })

watch(
  authBlocking,
  (blocking) => {
    if (blocking) authDelayed.setBusy(true)
    else authDelayed.reset()
  },
  { immediate: true },
)

const showShellSkeleton = computed(
  () => !bareLayout.value && (authDelayed.showUi.value || routePending.showUi.value),
)

const canShowProtected = computed(
  () => !bareLayout.value && !!auth.ready.value && !!auth.user.value && !routePending.showUi.value,
)

watch(locale, () => {
  updateDocumentTitle(route.meta.titleKey as string | undefined)
})
</script>

<template>
  <AppShell v-if="!bareLayout">
    <AppSkeleton v-if="showShellSkeleton" />
    <router-view v-else-if="canShowProtected" />
  </AppShell>
  <router-view v-else />
  <ToastHost />
  <div class="sr-only" role="status" aria-live="polite" aria-atomic="true" data-testid="loading-live">
    {{ liveMessage }}
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppSidebar from './AppSidebar.vue'
import AppTopbar from './AppTopbar.vue'
import AppSidebarNav from './AppSidebarNav.vue'
import BrandLogo from './BrandLogo.vue'
import Icon from '../ui/Icon.vue'
import {
  drainToast,
  formatGrace,
  isDraining,
  isOffline,
  shutdownState,
  startShutdownPolling,
  stopShutdownPolling,
} from '@/lib/composables/useShutdownState'
import { useAuth } from '@/lib/composables/useAuth'
import { useRefreshChrome } from '@/lib/shared/refreshChrome'
import { useRoutePending } from '@/lib/shared/routePending'

const route = useRoute()
const { t } = useI18n()
const full = computed(() => route.meta.full === true)
const draining = computed(() => isDraining())
const offline = computed(() => isOffline())
const auth = useAuth()
const refresh = useRefreshChrome()
const routePending = useRoutePending()

const showRefreshBar = computed(() => refresh.showTopBar.value)
const dimContent = computed(() => refresh.dimContent.value)
const mainAriaBusy = computed(() => {
  if (!auth.ready.value || !auth.user.value) return true
  if (routePending.pending.value || routePending.showUi.value) return true
  return refresh.ariaBusy.value
})

const drawerOpen = ref(false)

watch(
  () => route.path,
  () => {
    drawerOpen.value = false
  },
)

function closeDrawer() {
  drawerOpen.value = false
}

function toggleDrawer() {
  drawerOpen.value = !drawerOpen.value
}

onMounted(() => startShutdownPolling(4000))
onUnmounted(() => stopShutdownPolling())
</script>

<template>
  <div class="relative flex h-screen w-screen overflow-hidden bg-base text-txt">
    <AppSidebar />

    <div class="flex min-w-0 flex-1 flex-col">
      <div
        v-if="draining && !full"
        class="flex shrink-0 items-center gap-3 border-b border-warn/35 bg-warn/10 px-6 py-2 text-sm text-warn"
      >
        <span class="inline-flex h-2 w-2 animate-pulse rounded-full bg-warn" />
        <strong class="font-semibold text-txt">{{ t('common.shutdown.shuttingDown') }}</strong>
        <span class="text-txt2">{{ shutdownState.message || t('common.shutdown.notAcceptingRequests') }}</span>
        <span class="ml-auto font-mono text-xs text-txt2">
          {{ t('common.shutdown.graceRemaining', { time: formatGrace(shutdownState.graceRemainingSeconds) }) }}
        </span>
      </div>

      <AppTopbar v-if="!full" @toggle-menu="toggleDrawer" />
      <div
        v-if="!full && showRefreshBar"
        class="app-refresh-track"
        data-testid="app-refresh-bar"
        aria-hidden="true"
      >
        <div class="app-refresh-bar" />
      </div>
      <main
        class="relative min-h-0 flex-1 overflow-hidden"
        :aria-busy="mainAriaBusy ? 'true' : 'false'"
      >
        <div
          v-if="full && showRefreshBar"
          class="app-refresh-track"
          data-testid="app-refresh-bar"
          aria-hidden="true"
        >
          <div class="app-refresh-bar" />
        </div>
        <div v-if="full" class="h-full" :class="{ 'app-refresh-dim': dimContent }">
          <slot />
        </div>
        <div v-else class="scroll-area safe-area-bottom h-full min-h-0 overflow-y-auto bg-base">
          <div
            class="flex h-full min-h-0 flex-col px-4 py-4 md:px-6 md:py-6"
            :class="{ 'app-refresh-dim': dimContent }"
          >
            <slot />
          </div>
        </div>

        <div
          v-if="offline"
          class="absolute inset-0 z-40 flex items-center justify-center bg-base/75 backdrop-blur-sm"
        >
          <div class="border border-line bg-surface px-8 py-6 text-center shadow-card">
            <Icon name="alert" :size="28" class="mx-auto mb-3 text-txt3" />
            <h4 class="text-base font-semibold">{{ t('common.shutdown.offlineTitle') }}</h4>
            <p class="mt-2 text-sm text-txt3">{{ t('common.shutdown.offlineDesc') }}</p>
          </div>
        </div>
      </main>
    </div>

    <div
      v-if="drainToast.visible"
      class="pointer-events-none fixed bottom-6 right-6 z-50 max-w-sm border border-err/40 bg-elevated px-4 py-3 text-sm text-txt2 shadow-card"
    >
      <strong class="mb-1 block font-semibold text-err">{{ t('common.shutdown.actionUnavailable') }}</strong>
      {{ drainToast.text }}
    </div>

    <!-- Mobile drawer overlay -->
    <Teleport to="body">
      <Transition name="drawer-fade">
        <div
          v-if="drawerOpen"
          class="fixed inset-0 z-40 bg-black/50 md:hidden"
          @click="closeDrawer"
        />
      </Transition>
      <Transition name="drawer-slide">
        <aside
          v-if="drawerOpen"
          class="fixed inset-y-0 left-0 z-50 flex w-[min(280px,85vw)] flex-col border-r border-line bg-surface shadow-drawer md:hidden"
        >
          <div class="safe-area-top flex h-14 items-center justify-between gap-2 px-4">
            <BrandLogo />
            <button
              class="flex h-11 w-11 items-center justify-center rounded-md text-txt2 hover:bg-elevated hover:text-txt"
              :aria-label="t('shell.aria.closeNav')"
              @click="closeDrawer"
            >
              <Icon name="close" :size="18" />
            </button>
          </div>
          <AppSidebarNav @navigate="closeDrawer" />
        </aside>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 0.25s ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(-100%);
}
</style>

import type { Router } from 'vue-router'
import { computed, ref, watch } from 'vue'
import { i18n } from './i18n'
import { announceLoading, clearLoadingAnnounce } from './loadingAnnouncer'
import { createDelayedBusy } from './useDelayedBusy'

const pending = ref(false)
/** Route handover collapses immediately on afterEach — no minVisible flash after mount. */
const delayed = createDelayedBusy({ mode: 'initial', minVisibleMs: 0 })

watch(
  () => delayed.showUi.value,
  (ui) => {
    if (ui) announceLoading(String(i18n.global.t('common.loading.inProgress')))
    else clearLoadingAnnounce()
  },
)

export function beginRoutePending() {
  pending.value = true
  delayed.setBusy(true)
}

export function endRoutePending() {
  pending.value = false
  delayed.reset()
}

export function resetRoutePending() {
  pending.value = false
  delayed.reset()
}

export function useRoutePending() {
  return {
    pending: computed(() => pending.value),
    showUi: computed(() => delayed.showUi.value),
    busy: computed(() => delayed.busy.value),
  }
}

/**
 * Navigation pending engine: beforeEach sets pending, afterEach/onError clears it.
 * Faster than 200ms keeps the old page; after 200ms App.vue swaps main to skeleton.
 */
export function installRoutePendingGuards(router: Router) {
  router.beforeEach((to, from) => {
    if (from.matched.length > 0 && to.name === from.name && to.path === from.path) {
      return true
    }
    beginRoutePending()
    return true
  })
  router.afterEach(() => {
    endRoutePending()
  })
  router.onError(() => {
    endRoutePending()
  })
}

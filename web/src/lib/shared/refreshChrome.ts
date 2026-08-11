import { computed, ref, watch } from 'vue'
import { i18n } from './i18n'
import { announceLoading, clearLoadingAnnounce } from './loadingAnnouncer'
import type { RefreshIntent } from './loadingTypes'
import { createDelayedBusy } from '../composables/useDelayedBusy'
import { useToast } from '../composables/useToast'

const intent = ref<RefreshIntent | null>(null)
const rawBusy = ref(false)
const delayed = createDelayedBusy({ mode: 'refresh' })

let stickyToastId: number | null = null

const showChrome = computed(() => delayed.showUi.value && intent.value === 'user_initiated')

watch(showChrome, (show) => {
  const toast = useToast()
  if (show) {
    const msg = String(i18n.global.t('common.loading.refreshing'))
    stickyToastId = toast.showSticky(msg)
    announceLoading(msg)
    return
  }
  if (stickyToastId != null) {
    toast.dismiss(stickyToastId)
    stickyToastId = null
  }
  clearLoadingAnnounce()
})

watch(
  () => delayed.showUi.value,
  (ui) => {
    if (!ui && !rawBusy.value) intent.value = null
  },
)

export function beginRefresh(refreshIntent: RefreshIntent) {
  intent.value = refreshIntent
  rawBusy.value = true
  if (refreshIntent === 'user_initiated') {
    delayed.setBusy(true)
  }
}

export function endRefresh() {
  rawBusy.value = false
  delayed.setBusy(false)
  if (intent.value !== 'user_initiated') {
    intent.value = null
  }
}

export function resetRefreshChrome() {
  delayed.reset()
  rawBusy.value = false
  intent.value = null
  if (stickyToastId != null) {
    useToast().dismiss(stickyToastId)
    stickyToastId = null
  }
  clearLoadingAnnounce()
}

export function useRefreshChrome() {
  return {
    intent: computed(() => intent.value),
    ariaBusy: computed(() => rawBusy.value),
    showTopBar: computed(() => showChrome.value),
    dimContent: computed(() => showChrome.value),
    showUi: computed(() => delayed.showUi.value),
  }
}

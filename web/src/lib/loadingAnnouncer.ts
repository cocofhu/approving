import { ref } from 'vue'

const liveMessage = ref('')

/** Polite live region text. Silent peek/poll must not call this. */
export function announceLoading(msg: string) {
  liveMessage.value = ''
  queueMicrotask(() => {
    liveMessage.value = msg
  })
}

export function clearLoadingAnnounce() {
  liveMessage.value = ''
}

export function useLoadingAnnouncer() {
  return { liveMessage }
}

export function resetLoadingAnnouncer() {
  liveMessage.value = ''
}

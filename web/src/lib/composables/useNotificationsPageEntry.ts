import { ref } from 'vue'

/**
 * Same-route entry signal for /notifications.
 * Incrementing while already on the page resets filter=all and page=1
 * without writing page into the URL.
 */
const enterNonce = ref(0)

export function requestNotificationsPageReset() {
  enterNonce.value += 1
}

export function useNotificationsPageEntry() {
  return { enterNonce }
}

export function __resetNotificationsPageEntryForTests() {
  enterNonce.value = 0
}

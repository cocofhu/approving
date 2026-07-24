import { ref } from 'vue'

export type ToastType = 'default' | 'success' | 'error' | 'warn'

export interface ToastItem {
  id: number
  message: string
  type: ToastType
}

let idCounter = 0
const toasts = ref<ToastItem[]>([])

export function useToast() {
  function show(message: string, type: ToastType = 'default') {
    const id = ++idCounter
    toasts.value.push({ id, message, type })
    setTimeout(() => {
      toasts.value = toasts.value.filter((t) => t.id !== id)
    }, 2600)
  }

  return {
    toasts,
    show,
    success: (message: string) => show(message, 'success'),
    error: (message: string) => show(message, 'error'),
    warn: (message: string) => show(message, 'warn'),
  }
}

import { describe, expect, it, vi } from 'vitest'
import { useToast } from './useToast'

describe('useToast', () => {
  it('pushes and auto-dismisses toast entries', () => {
    vi.useFakeTimers()
    const toast = useToast()
    toast.toasts.value = []
    toast.success('ok')
    toast.error('err')
    toast.warn('warn')
    toast.show('info')
    expect(toast.toasts.value).toHaveLength(4)
    vi.advanceTimersByTime(3000)
    expect(toast.toasts.value).toHaveLength(0)
    vi.useRealTimers()
  })
})

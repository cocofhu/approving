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

  it('sticky toast stays until dismiss', () => {
    vi.useFakeTimers()
    const toast = useToast()
    toast.toasts.value = []
    const id = toast.showSticky('正在刷新')
    expect(toast.toasts.value).toHaveLength(1)
    expect(toast.toasts.value[0].sticky).toBe(true)
    vi.advanceTimersByTime(5000)
    expect(toast.toasts.value).toHaveLength(1)
    toast.dismiss(id)
    expect(toast.toasts.value).toHaveLength(0)
    vi.useRealTimers()
  })
})

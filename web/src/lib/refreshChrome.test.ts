import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { beginRefresh, endRefresh, resetRefreshChrome, useRefreshChrome } from './refreshChrome'
import { resetLoadingAnnouncer, useLoadingAnnouncer } from './loadingAnnouncer'
import { useToast } from './useToast'

describe('refreshChrome', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetRefreshChrome()
    resetLoadingAnnouncer()
    useToast().toasts.value = []
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('user_initiated shows top bar, dim, sticky toast and live after 200ms', async () => {
    beginRefresh('user_initiated')
    const chrome = useRefreshChrome()
    expect(chrome.ariaBusy.value).toBe(true)
    expect(chrome.showTopBar.value).toBe(false)
    expect(chrome.dimContent.value).toBe(false)
    await vi.advanceTimersByTimeAsync(200)
    await nextTick()
    expect(chrome.showTopBar.value).toBe(true)
    expect(chrome.dimContent.value).toBe(true)
    expect(useToast().toasts.value.some((t) => t.sticky)).toBe(true)
    await Promise.resolve()
    expect(useLoadingAnnouncer().liveMessage.value).toBeTruthy()
    endRefresh()
    expect(chrome.ariaBusy.value).toBe(false)
  })

  it('silent_poll only sets aria-busy — no toast, bar, dim, or live', async () => {
    beginRefresh('silent_poll')
    const chrome = useRefreshChrome()
    expect(chrome.ariaBusy.value).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    await nextTick()
    expect(chrome.showTopBar.value).toBe(false)
    expect(chrome.dimContent.value).toBe(false)
    expect(useToast().toasts.value).toHaveLength(0)
    expect(useLoadingAnnouncer().liveMessage.value).toBe('')
    endRefresh()
    expect(chrome.ariaBusy.value).toBe(false)
  })
})

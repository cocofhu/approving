import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import {
  beginRoutePending,
  endRoutePending,
  installRoutePendingGuards,
  resetRoutePending,
  useRoutePending,
} from './routePending'
import { resetLoadingAnnouncer, useLoadingAnnouncer } from './loadingAnnouncer'

describe('routePending engine', () => {
  it('does not show UI before 200ms and reset collapses immediately', async () => {
    vi.useFakeTimers()
    resetRoutePending()
    beginRoutePending()
    const rp = useRoutePending()
    expect(rp.pending.value).toBe(true)
    expect(rp.showUi.value).toBe(false)
    await vi.advanceTimersByTimeAsync(200)
    expect(rp.showUi.value).toBe(true)
    endRoutePending()
    expect(rp.pending.value).toBe(false)
    expect(rp.showUi.value).toBe(false)
    vi.useRealTimers()
  })

  it('afterEach clears pending after navigation completes', async () => {
    resetRoutePending()
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/a', name: 'a', component: { template: '<div />' } },
        { path: '/b', name: 'b', component: { template: '<div />' } },
      ],
    })
    installRoutePendingGuards(router)
    beginRoutePending()
    expect(useRoutePending().pending.value).toBe(true)
    await router.push('/a')
    expect(useRoutePending().pending.value).toBe(false)
    expect(useRoutePending().showUi.value).toBe(false)
  })

  it('announces polite loading when delayed UI shows, not before', async () => {
    vi.useFakeTimers()
    resetRoutePending()
    resetLoadingAnnouncer()
    beginRoutePending()
    expect(useLoadingAnnouncer().liveMessage.value).toBe('')
    await vi.advanceTimersByTimeAsync(200)
    await Promise.resolve()
    expect(useLoadingAnnouncer().liveMessage.value).toBeTruthy()
    endRoutePending()
    vi.useRealTimers()
  })
})

// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  drainToast,
  formatGrace,
  isDraining,
  isMutationMethod,
  isOffline,
  mutationsBlocked,
  pollShutdownHealth,
  showDrainToast,
  shutdownState,
  startShutdownPolling,
  stopShutdownPolling,
} from './useShutdownState'
import { serviceCommit } from './useServiceCommit'

describe('useShutdownState', () => {
  beforeEach(() => {
    stopShutdownPolling()
    shutdownState.mode = 'normal'
    shutdownState.message = ''
    shutdownState.graceRemainingSeconds = 0
    shutdownState.checked = false
    drainToast.visible = false
    serviceCommit.value = ''
    vi.useFakeTimers()
  })

  afterEach(() => {
    stopShutdownPolling()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('classifies modes and mutation methods', () => {
    expect(isMutationMethod('post')).toBe(true)
    expect(isMutationMethod('GET')).toBe(false)
    expect(mutationsBlocked()).toBe(false)
    shutdownState.mode = 'draining'
    expect(isDraining()).toBe(true)
    expect(mutationsBlocked()).toBe(true)
    shutdownState.mode = 'offline'
    expect(isOffline()).toBe(true)
    expect(formatGrace(125)).toBe('2:05')
  })

  it('polls health into draining/normal/offline and shows toast', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ status: 'shutting_down', message: 'bye', grace_remaining_seconds: 30 }), {
        status: 503,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await pollShutdownHealth()
    expect(shutdownState.mode).toBe('draining')
    expect(shutdownState.graceRemainingSeconds).toBe(30)

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ status: 'ok' }), { status: 200 }))
    await pollShutdownHealth()
    expect(shutdownState.mode).toBe('normal')

    fetchMock.mockResolvedValueOnce(new Response('', { status: 500 }))
    await pollShutdownHealth()
    expect(shutdownState.mode).toBe('offline')

    fetchMock.mockRejectedValueOnce(new Error('net'))
    await pollShutdownHealth()
    expect(shutdownState.mode).toBe('offline')

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ status: 'ok', ready: true, commit: 'B01BB39deadbeef' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await pollShutdownHealth()
    expect(shutdownState.mode).toBe('normal')
    expect(serviceCommit.value).toBe('b01bb39')

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ status: 'ok', ready: true }), { status: 200 }))
    await pollShutdownHealth()
    expect(serviceCommit.value).toBe('')

    showDrainToast('blocked')
    expect(drainToast.visible).toBe(true)
    expect(drainToast.text).toBe('blocked')
    vi.advanceTimersByTime(4000)
    expect(drainToast.visible).toBe(false)

    startShutdownPolling(1000)
    startShutdownPolling(1000)
    expect(fetchMock).toHaveBeenCalled()
    stopShutdownPolling()
  })
})

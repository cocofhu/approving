import { describe, expect, it, vi } from 'vitest'
import { createBusySeedRetryController, runBusySeedRetry } from './busySeedRetry'

describe('runBusySeedRetry', () => {
  it('returns content when first seed succeeds', async () => {
    const seed = vi.fn(async () => true)
    const reason = await runBusySeedRetry({
      isBusy: () => true,
      hasContent: () => false,
      seed,
      intervalMs: 1,
    })
    expect(reason).toBe('content')
    expect(seed).toHaveBeenCalledTimes(1)
  })

  it('retries while busy until content appears', async () => {
    let calls = 0
    const seed = vi.fn(async () => {
      calls += 1
      return calls >= 3
    })
    const reason = await runBusySeedRetry({
      isBusy: () => true,
      hasContent: () => false,
      seed,
      intervalMs: 1,
      maxAttempts: 10,
    })
    expect(reason).toBe('content')
    expect(seed).toHaveBeenCalledTimes(3)
  })

  it('stops when authority becomes non-busy', async () => {
    let busy = true
    const seed = vi.fn(async () => {
      busy = false
      return false
    })
    const reason = await runBusySeedRetry({
      isBusy: () => busy,
      hasContent: () => false,
      seed,
      intervalMs: 1,
    })
    expect(reason).toBe('idle')
  })

  it('stops when live incremental arrives', async () => {
    let live = false
    const seed = vi.fn(async () => {
      live = true
      return false
    })
    const reason = await runBusySeedRetry({
      isBusy: () => true,
      hasContent: () => false,
      liveIncrementalReceived: () => live,
      seed,
      intervalMs: 1,
    })
    expect(reason).toBe('live')
  })

  it('aborts when signal fires', async () => {
    const ctrl = new AbortController()
    const seed = vi.fn(async () => {
      ctrl.abort()
      return false
    })
    const reason = await runBusySeedRetry({
      isBusy: () => true,
      hasContent: () => false,
      seed,
      signal: ctrl.signal,
      intervalMs: 50,
    })
    expect(reason).toBe('aborted')
  })
})

describe('createBusySeedRetryController', () => {
  it('aborts previous loop when start is called again', async () => {
    const c = createBusySeedRetryController()
    let firstAborted = false
    c.start(async (signal) => {
      await new Promise<void>((resolve) => {
        signal.addEventListener('abort', () => {
          firstAborted = true
          resolve()
        })
      })
    })
    c.start(async () => undefined)
    expect(firstAborted).toBe(true)
    c.stop()
  })
})

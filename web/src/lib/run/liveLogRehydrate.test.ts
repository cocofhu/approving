import { describe, expect, it, vi } from 'vitest'
import {
  allowBootEmptyState,
  REHYDRATE_TIMEOUT_MS,
  RehydrateOrchestrator,
  resolveRehydrateAfterFetch,
} from './liveLogRehydrate'

describe('liveLogRehydrate', () => {
  it('keeps rehydrate timeout near 10s and separate from Boot 120s', () => {
    expect(REHYDRATE_TIMEOUT_MS).toBe(10_000)
    expect(REHYDRATE_TIMEOUT_MS).toBeLessThan(120_000)
  })

  it('allows Boot empty-wait only after successful rehydrate', () => {
    expect(allowBootEmptyState('ready')).toBe(true)
    expect(allowBootEmptyState('loading')).toBe(false)
    expect(allowBootEmptyState('error')).toBe(false)
    expect(allowBootEmptyState('idle')).toBe(false)
    expect(allowBootEmptyState(undefined)).toBe(false)
  })

  it('resolveRehydrateAfterFetch ignores stale gen and non-loading status', () => {
    expect(resolveRehydrateAfterFetch('loading', 1, 1, 'ok')).toBe('ready')
    expect(resolveRehydrateAfterFetch('loading', 1, 1, 'error')).toBe('error')
    expect(resolveRehydrateAfterFetch('loading', 1, 2, 'error')).toBeNull()
    expect(resolveRehydrateAfterFetch('error', 1, 1, 'ok')).toBeNull()
    expect(resolveRehydrateAfterFetch('loading', 1, 1, 'stale')).toBeNull()
  })
})

describe('RehydrateOrchestrator race / abort', () => {
  function deferred<T>() {
    let resolve!: (v: T) => void
    let reject!: (e: unknown) => void
    const promise = new Promise<T>((res, rej) => {
      resolve = res
      reject = rej
    })
    return { promise, resolve, reject }
  }

  it('timeout flips loading→error and aborts in-flight fetch', async () => {
    const timers: Array<{ fn: () => void; ms: number }> = []
    const fetch = vi.fn(async (signal: AbortSignal) => {
      await new Promise<void>((resolve, reject) => {
        signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
        // never resolves unless aborted
        void resolve
      })
      return 'ok' as const
    })

    const orch = new RehydrateOrchestrator({
      timeoutMs: 10,
      fetch,
      schedule: (fn, ms) => {
        timers.push({ fn, ms })
        return { clear: () => {} }
      },
    })

    const runP = orch.run({ running: true })
    expect(orch.status).toBe('loading')
    expect(timers).toHaveLength(1)
    expect(timers[0].ms).toBe(10)

    timers[0].fn()
    expect(orch.status).toBe('error')

    await runP
    expect(orch.status).toBe('error')
    orch.dispose()
  })

  it('hang → timeout → retry → success: old attempt cannot spoil new loading', async () => {
    const timers: Array<{ fn: () => void }> = []
    let fetchN = 0
    const first = deferred<'ok' | 'error'>()
    const second = deferred<'ok' | 'error'>()

    const orch = new RehydrateOrchestrator({
      timeoutMs: 10,
      fetch: async (signal) => {
        const n = ++fetchN
        const d = n === 1 ? first : second
        return await new Promise<'ok' | 'error'>((resolve, reject) => {
          signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
          d.promise.then(resolve, reject)
        })
      },
      schedule: (fn) => {
        timers.push({ fn })
        return { clear: () => {} }
      },
    })

    const firstRun = orch.run({ running: true })
    expect(orch.status).toBe('loading')
    expect(fetchN).toBe(1)

    // ~10s hang → error + abort first request
    timers[0].fn()
    expect(orch.status).toBe('error')
    await firstRun
    expect(orch.status).toBe('error')

    // Manual retry while (hypothetically) old work could still settle
    const retryRun = orch.run({ running: true, force: true })
    expect(orch.status).toBe('loading')
    expect(fetchN).toBe(2)

    // Late "failure" from gen-1 must not win (already aborted/stale).
    first.resolve('error')
    await Promise.resolve()
    expect(orch.status).toBe('loading')

    second.resolve('ok')
    await retryRun
    expect(orch.status).toBe('ready')
    orch.dispose()
  })

  it('force retry before timeout cancels prior request and accepts new success', async () => {
    const timers: Array<{ fn: () => void; clear: () => void }> = []
    let fetchN = 0
    const first = deferred<'ok' | 'error'>()
    const second = deferred<'ok' | 'error'>()

    const orch = new RehydrateOrchestrator({
      timeoutMs: 10_000,
      fetch: async (signal) => {
        const n = ++fetchN
        const d = n === 1 ? first : second
        return await new Promise<'ok' | 'error'>((resolve, reject) => {
          signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
          d.promise.then(resolve, reject)
        })
      },
      schedule: (fn) => {
        const entry = { fn, clear: vi.fn() }
        timers.push(entry)
        return { clear: () => entry.clear() }
      },
    })

    const firstRun = orch.run({ running: true })
    expect(orch.status).toBe('loading')

    const retryRun = orch.run({ running: true, force: true })
    expect(orch.status).toBe('loading')
    expect(fetchN).toBe(2)

    first.resolve('ok') // stale success — must be ignored
    await firstRun
    expect(orch.status).toBe('loading')

    second.resolve('ok')
    await retryRun
    expect(orch.status).toBe('ready')
    orch.dispose()
  })
})

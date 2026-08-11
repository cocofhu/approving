/**
 * Acceptance checklist (g5.2): Clarify / Gate / Run × 硬刷新 / 重进 / WS / 完成态.
 * Source-contract + unit helpers — full browser matrix is for e2e/manual.
 *
 * Soft-refresh semantics and「思考中…」copy must remain unchanged (g2.3).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import { deliverOrBufferDialogueAcp } from './dialogueAcpDelivery'
import { runBusySeedRetry } from './busySeedRetry'
import { createWsReconnectController } from './wsReconnect'
import type { AcpEvent } from '@/lib/shared/types'

const here = dirname(fileURLToPath(import.meta.url))

function readSrc(rel: string): string {
  return readFileSync(join(here, '../..', rel), 'utf8')
}

const sample: AcpEvent[] = [
  { kind: 'thought', text: 't', t: 1 },
  { kind: 'message', text: 'm', t: 2 },
]

describe('stream resume acceptance (g5.2 three surfaces × four scenarios)', () => {
  it('g1: false-applied → buffer on clarify/gate delivery helpers', () => {
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: true,
        forGate: false,
        events: sample,
        nodeId: 'c1',
        applyClarify: () => false,
      }),
    ).toBe('buffer')
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: false,
        forGate: true,
        events: sample,
        nodeId: 'g1',
        applyGate: () => false,
      }),
    ).toBe('buffer')
  })

  it('g2.1: Inbox/Run restore order is queue_state → seed → flush', () => {
    const inbox = readSrc('views/GatesInboxView.vue')
    const run = readSrc('views/RunDetailView.vue')
    const projectStart = inbox.indexOf('async function projectClarifySessionAfterLoad')
    expect(projectStart).toBeGreaterThan(-1)
    const projectSlice = inbox.slice(projectStart, projectStart + 1200)
    const inboxIdx = [
      projectSlice.indexOf('restoreReactSessions(r)'),
      projectSlice.indexOf('seedClarifyAcpFromNodeEventsOnce'),
      projectSlice.indexOf('flushPendingAcpFrames()'),
    ]
    expect(inboxIdx.every((i) => i >= 0)).toBe(true)
    expect(inboxIdx[0]).toBeLessThan(inboxIdx[1]!)
    expect(inboxIdx[1]).toBeLessThan(inboxIdx[2]!)
    expect(run).toMatch(/seedDialogueNodeOnce/)
    expect(run.indexOf('seedDialogueAcpAfterRestore')).toBeGreaterThan(-1)
    expect(run.indexOf('flushPendingDialogueAcp')).toBeGreaterThan(-1)
  })

  it('g2.3: soft refresh path and thinkingBusy copy untouched', () => {
    const inbox = readSrc('views/GatesInboxView.vue')
    const clarify = readSrc('components/run/ClarifyChat.vue')
    const gatePanel = readSrc('components/run/GateReactStreamPanel.vue')
    expect(inbox).toMatch(/isClarifySoftRefreshBlocked/)
    expect(inbox).toMatch(/Soft-refresh semantics unchanged/)
    expect(clarify).toMatch(/pages\.clarify\.thinkingBusy/)
    expect(gatePanel).toMatch(/pages\.clarify\.thinkingBusy/)
    expect(clarify).not.toMatch(/正在恢复对话/)
    expect(gatePanel).not.toMatch(/正在恢复对话/)
  })

  it('g3: busy seed retry stops on content / idle', async () => {
    let content = false
    const reason = await runBusySeedRetry({
      isBusy: () => true,
      hasContent: () => content,
      liveIncrementalReceived: () => false,
      intervalMs: 1,
      maxAttempts: 5,
      seed: async () => {
        content = true
        return true
      },
    })
    expect(reason).toBe('content')
    const idle = await runBusySeedRetry({
      isBusy: () => false,
      hasContent: () => false,
      seed: async () => false,
    })
    expect(idle).toBe('idle')
  })

  it('g4: WS reconnect controller schedules connect when wanted', () => {
    const connect = vi.fn()
    const ctrl = createWsReconnectController({
      connect,
      shouldReconnect: () => true,
      baseDelayMs: 1,
      maxDelayMs: 1,
      schedule: (fn) => {
        fn()
        return 1 as unknown as ReturnType<typeof setTimeout>
      },
      clearSchedule: () => undefined,
    })
    expect(ctrl.onClose()).toBe(true)
    expect(connect).toHaveBeenCalled()
  })

  it('Inbox + Run wire busySeedRetry and wsReconnect (g3/g4)', () => {
    const inbox = readSrc('views/GatesInboxView.vue')
    const run = readSrc('views/RunDetailView.vue')
    for (const src of [inbox, run]) {
      expect(src).toMatch(/createBusySeedRetryController/)
      expect(src).toMatch(/createWsReconnectController/)
      expect(src).toMatch(/fromReconnect/)
    }
    expect(inbox).toMatch(/projectClarifySessionAfterLoad/)
    expect(run).toMatch(/projectDialogueAfterLoad/)
  })
})

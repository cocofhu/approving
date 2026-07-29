// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'GatesInboxView.vue'), 'utf8')

describe('GatesInboxView GateApproval isolation', () => {
  it('keeps fill-preview but does not enable mobile-fill-remaining', () => {
    expect(src).toMatch(/:fill-preview="true"/)
    expect(src).not.toMatch(/mobile-fill-remaining/)
    expect(src).not.toMatch(/mobileFillRemaining/)
  })

  it('clarify stage uses loading pane, product panel, and load-failed retry', () => {
    expect(src).toMatch(/ArtifactLoadingPane/)
    expect(src).toMatch(/ClarifyProductStage/)
    expect(src).toMatch(/clarifyStageKind/)
    expect(src).toMatch(/retryActiveRun/)
    expect(src).toMatch(/clarifyProductNodes/)
  })
})

describe('GatesInboxView review/clarify composer mode', () => {
  it('derives reviewActive via inboxReviewMode helpers', () => {
    expect(src).toMatch(/resolveInboxReviewState/)
    expect(src).toMatch(/pickInboxClarifySession/)
    expect(src).toMatch(/inboxComposerMode/)
    expect(src).toMatch(/const reviewActive = computed/)
    expect(src).toMatch(/composerMode/)
  })

  it('binds dynamic mode and finish on both ReviewComposers', () => {
    expect(src).not.toMatch(/mode="clarify"/)
    const modeBindings = src.match(/:mode="composerMode"/g) || []
    expect(modeBindings.length).toBe(2)
    const finishBindings = src.match(/@finish="onClarifyFinish"/g) || []
    expect(finishBindings.length).toBe(2)
  })

  it('finish uses confirmFlowPrompt with force=true and success/error toasts', () => {
    expect(src).toMatch(/function onClarifyFinish/)
    expect(src).toMatch(/pages\.clarify\.confirmFlowPrompt/)
    expect(src).toMatch(/onClarifySend\(prompt, \[\], \[\], true\)/)
    expect(src).toMatch(/pages\.gatesInbox\.reviewFinished/)
    expect(src).toMatch(/pages\.gatesInbox\.reviewFinishedPending/)
    expect(src).toMatch(/pages\.gatesInbox\.reviewFinishFailed/)
  })
})

describe('GatesInboxView app_preview stage (g2.2)', () => {
  it('mounts AppPreviewPanel with fill + pick wiring on both ReviewShell stages', () => {
    expect(src).toMatch(/import AppPreviewPanel/)
    expect(src).toMatch(/inboxAppPreviewActive/)
    expect(src).toMatch(/addClarifyAnnotation/)
    expect(src).toMatch(/mergeStagedAppPreviewPick/)
    const panelBlocks = src.match(/<AppPreviewPanel[\s\S]*?:show-feedback="false"/g) || []
    expect(panelBlocks.length).toBe(2)
    expect(src).toMatch(/@pick="onAppPreviewReviewPick"/)
    expect(src).toMatch(/@staged-pick="onAppPreviewStagedPick"/)
  })
})

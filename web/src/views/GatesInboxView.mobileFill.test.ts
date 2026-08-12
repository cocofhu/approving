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

  it('desktop Inbox enables unified-preview-budget; mobile detail does not', () => {
    // plan g2.1: Inbox desktop fill-preview path only
    expect(src).toMatch(/:unified-preview-budget="true"/)
    // Desktop grid stretch (g1.1) — not items-start
    expect(src).toMatch(/items-stretch/)
    expect(src).not.toMatch(/grid-cols-\[320px_1fr\] items-start/)
    // Mobile GateApproval block omits unified budget (only one binding, on desktop).
    const unifiedBindings = src.match(/:unified-preview-budget="true"/g) || []
    expect(unifiedBindings.length).toBe(1)
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

const EMPTY_CARD_CLASS =
  'card flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto'

describe('GatesInboxView empty inbox fill (plan g1 / g2.1 / g1.3)', () => {
  it('mobile + desktop empty wrappers both include flex-1 and vertical centering (g1.1 g1.2 g2.1)', () => {
    const escaped = EMPTY_CARD_CLASS.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const matches = src.match(new RegExp(`class="${escaped}"`, 'g')) || []
    expect(matches.length).toBe(2)
    expect(src).toMatch(/items-center justify-center overflow-auto/)
    expect(src).not.toMatch(/<div v-else class="card">/)
  })

  it('pipeline-filter empty and global empty share the same fill wrappers (g1.3)', () => {
    const blocks = [
      ...src.matchAll(
        /<div v-else class="card flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto">[\s\S]*?<\/div>/g,
      ),
    ]
    expect(blocks.length).toBe(2)
    for (const block of blocks) {
      expect(block[0]).toMatch(/listTotal \? t\('common\.empty\.noPendingGatesForPipeline'\)/)
      expect(block[0]).toMatch(/listTotal\s+\?\s+t\('common\.empty\.noPendingGatesPipelineDesc'\)/)
      expect(block[0]).toMatch(/t\('common\.empty\.noPendingGates'\)/)
      expect(block[0]).toMatch(/t\('common\.empty\.noPendingGatesDesc'\)/)
    }
  })

  it('empty wrappers keep .card skin tokens and do not zero-radius (g3.1)', () => {
    const matches = src.match(/class="card flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto"/g) || []
    expect(matches.length).toBe(2)
    expect(src).not.toMatch(/inbox-empty.*rounded-none/)
    expect(src).not.toMatch(/empty-card.*!rounded-none/)
  })
})

describe('GatesInboxView list first-load tri-state (plan g1 / g2 / g3.2)', () => {
  it('listLoading starts true and template consumes listLoading + listLoadFailed', () => {
    expect(src).toMatch(/const listLoading = ref\(true\)/)
    expect(src).toMatch(/const listLoadFailed = ref\(false\)/)
    expect(src).toMatch(/showListSkeleton/)
    expect(src).toMatch(/showListError/)
    expect(src).toMatch(/listLoadFailed\.value = true/)
    expect(src).toMatch(/listLoadFailed\.value = false/)
    expect(src).toMatch(/retryListLoad/)
    expect(src).toMatch(/t\('pages\.gatesInbox\.listLoadingHint'\)/)
    expect(src).toMatch(/t\('common\.asyncState\.loadFailedTitle'\)/)
    expect(src).toMatch(/t\('common\.asyncState\.loadFailedDesc'\)/)
    expect(src).toMatch(/t\('common\.buttons\.retry'\)/)
    expect(src).toMatch(/data-testid="inbox-list-skeleton"/)
    expect(src).toMatch(/data-testid="inbox-list-failed"/)
    expect(src).toMatch(/data-testid="inbox-pending-card-skeleton"/)
    expect(src).toMatch(/aria-busy="true"/)
    expect(src).toMatch(/LIST_SKELETON_COUNT = 6/)
    expect(src).toMatch(/app-skeleton__block h-9 w-9/)
    expect(src).not.toMatch(/<AppSkeleton/)
    expect(src).not.toMatch(/InboxPendingCardSkeleton/)
  })

  it('branch order is loading∧empty → error∧empty → EmptyState; empty wrappers stay two', () => {
    const mobileList = src.slice(src.indexOf('<!-- Mobile list view -->'), src.indexOf('<!-- Mobile detail view -->'))
    expect(mobileList.indexOf('v-else-if="showListSkeleton"')).toBeGreaterThan(-1)
    expect(mobileList.indexOf('v-else-if="showListError"')).toBeGreaterThan(
      mobileList.indexOf('v-else-if="showListSkeleton"'),
    )
    expect(mobileList.indexOf('v-else class="card flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto"')).toBeGreaterThan(
      mobileList.indexOf('v-else-if="showListError"'),
    )

    const desktop = src.slice(src.indexOf('<!-- Desktop loading'))
    expect(desktop.indexOf('v-else-if="!isMobile && showListSkeleton"')).toBeGreaterThan(-1)
    expect(desktop.indexOf('v-else-if="!isMobile && showListError"')).toBeGreaterThan(
      desktop.indexOf('v-else-if="!isMobile && showListSkeleton"'),
    )
    expect(desktop.indexOf('v-else-if="!isMobile && listItems.length"')).toBeGreaterThan(
      desktop.indexOf('v-else-if="!isMobile && showListError"'),
    )
    expect(desktop).toMatch(/grid-cols-\[320px_1fr\] items-stretch/)
    expect(src).toMatch(/loadList\(\{ showLoading: listItems\.value\.length === 0 \}\)/)
    expect(src).toMatch(/void loadList\(\{ showLoading: true \}\)/)
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

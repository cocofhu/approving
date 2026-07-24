// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'RunDetailView.vue'), 'utf8')

describe('RunDetailView GateApproval fill-preview', () => {
  it('always passes fill-preview=true (desktop content-fit), not isMobile', () => {
    expect(src).toMatch(/<GateApproval[\s\S]*?:fill-preview="true"/)
    expect(src).not.toMatch(/:fill-preview="isMobile"/)
    expect(src).not.toMatch(/fit-visual-preview/)
    expect(src).not.toMatch(/fitVisualPreview/)
  })

  it('passes mobile-fill-remaining for Run-detail visual layout isolation', () => {
    expect(src).toMatch(/<GateApproval[\s\S]*?:mobile-fill-remaining="true"/)
    expect(src).toMatch(/data-testid="run-detail-right-panel"/)
    expect(src).toMatch(/min-h-0 flex-1/)
  })
})

describe('RunDetailView priority badge popover', () => {
  it('does not render a default-expanded editor under the progress bar', () => {
    // Progress bar block must not be followed by an always-mounted priority editor card.
    expect(src).not.toMatch(
      /progressFrac \* 100 \+ '%'[\s\S]{0,400}data-testid="run-priority-editor"/,
    )
    // Editor lives in a Teleport popover gated by priorityPopoverOpen.
    expect(src).toMatch(/priorityPopoverOpen/)
    expect(src).toMatch(/<Teleport to="body">/)
    expect(src).toMatch(/data-testid="run-priority-editor"/)
    expect(src).toMatch(/data-testid="priority-popover-backdrop"/)
  })

  it('uses an editable badge trigger with aria switch attributes', () => {
    expect(src).toMatch(/data-testid="priority-badge"/)
    expect(src).toMatch(/aria-haspopup="dialog"/)
    expect(src).toMatch(/:aria-expanded="priorityPopoverOpen"/)
    expect(src).toMatch(/aria-controls="run-priority-editor"/)
    expect(src).toMatch(/priorityEditTrigger/)
    expect(src).toMatch(/priorityEditAria/)
    // Inner badge must not override the trigger hover title.
    expect(src).toMatch(/<PriorityBadge :priority="run\.priority" hide-title/)
  })

  it('shows chevron only for queued and supports Esc / backdrop close without cancel', () => {
    expect(src).toMatch(/showPriorityChevron/)
    expect(src).toMatch(/run\.value\.status === 'queued'/)
    expect(src).toMatch(/name="chevron-down"/)
    expect(src).toMatch(/priorityChevronClass/)
    expect(src).toMatch(/function closePriorityPopover\(/)
    expect(src).toMatch(/e\.key === 'Escape'/)
    expect(src).toMatch(/@click="closePriorityPopover\(true\)"/)
    // No explicit cancel button in the editor layer.
    expect(src).not.toMatch(/run-priority-editor[\s\S]{0,800}cancel/i)
  })

  it('moves focus into the dialog on open and returns it to the trigger on close', () => {
    expect(src).toMatch(/ref="priorityEditorRef"/)
    expect(src).toMatch(/tabindex="-1"/)
    expect(src).toMatch(/priorityEditorRef\.value\?\.focus\(\)/)
    expect(src).toMatch(/priorityBadgeRef\.value\?\.focus\(\)/)
  })

  it('keeps success tip independent from draft sync and auto-closes after save', () => {
    expect(src).toMatch(/function showPrioritySaved\(/)
    expect(src).toMatch(/priorityOkTimer = setTimeout/)
    expect(src).toMatch(/closePriorityPopover\(false\)/)
    // syncPriorityDraft must not clear the success tip (would flash it away on save).
    const syncFn = src.match(/function syncPriorityDraft\(\) \{[\s\S]*?\n\}/)
    expect(syncFn?.[0]).toBeTruthy()
    expect(syncFn?.[0]).not.toMatch(/priorityOk\.value = false/)
  })

  it('discards draft on close and blocks save when draft equals committed', () => {
    expect(src).toMatch(/function closePriorityPopover\(discard = true\)/)
    expect(src).toMatch(/if \(discard\) syncPriorityDraft\(\)/)
    expect(src).toMatch(/if \(priorityDraft\.value === committedPriority\.value\) return/)
    expect(src).toMatch(/:disabled="prioritySaving \|\| priorityDraft === committedPriority"/)
  })
})

describe('RunDetailView priority draft vs poll', () => {
  it('watches scalar priority/status and preserves dirty draft while editable', () => {
    // Must watch scalar getters — `() => [priority, status]` returns a new array
    // each evaluation and re-fires on every poll that replaces run.value.
    expect(src).toMatch(
      /watch\(\s*\[\s*\(\)\s*=>\s*run\.value\.priority\s*,\s*\(\)\s*=>\s*run\.value\.status\s*\]/,
    )
    expect(src).not.toMatch(
      /watch\(\s*\(\)\s*=>\s*\[\s*run\.value\.priority\s*,\s*run\.value\.status\s*\]/,
    )
    expect(src).toMatch(
      /if \(priorityEditable\.value && priorityDraft\.value !== saved\) return/,
    )
  })
})

describe('RunDetailView live-log boot timeout persistence', () => {
  it('keeps log/sandbox panels mounted with v-show and persists boot session', () => {
    // log ↔ sandbox must not use exclusive v-else-if on LiveLogPanel alone
    // (that unmounted the panel and reset the ~120s dwell clock).
    expect(src).toMatch(/nodeTab === 'log' \|\| nodeTab === 'sandbox'/)
    expect(src).toMatch(/v-show="nodeTab === 'log'"/)
    expect(src).toMatch(/v-show="nodeTab === 'sandbox'"/)
    expect(src).toMatch(/liveLogBootSessions/)
    expect(src).toMatch(/:boot-session="currentLiveLogBootSession"/)
    expect(src).toMatch(/@boot-session="onLiveLogBootSession"/)
    // Remount on node/execution switch so ratchet state cannot leak across sessions.
    expect(src).toMatch(/:key="`\$\{selected\}:\$\{selExecIdx\}`"/)
    expect(src).toMatch(/function goSandboxLogTab\(\)/)
    expect(src).toMatch(/nodeTab\.value = 'sandbox'/)
  })

  it('polls sandbox signals for boot while on log or sandbox tab', () => {
    expect(src).toMatch(/nodeTab\.value !== 'log' && nodeTab\.value !== 'sandbox'/)
  })
})

describe('RunDetailView ACP log rehydrate state machine', () => {
  it('wires rehydrate loading/error/retry separate from Boot and default tab', () => {
    expect(src).toMatch(/RehydrateOrchestrator/)
    expect(src).toMatch(/async function rehydrateNodeEvents/)
    expect(src).toMatch(/function retryRehydrate/)
    expect(src).toMatch(/:rehydrate-status="selRehydrateStatus"/)
    expect(src).toMatch(/@retry-rehydrate="retryRehydrate"/)
    // Rehydrate runs on enter regardless of active tab; default tab logic unchanged.
    expect(src).toMatch(/void rehydrateNodeEvents\(selected\.value\)/)
    expect(src).toMatch(/else if \(hasLog\.value\) nodeTab\.value = 'log'/)
    // Failed rehydrate must not auto-recover via the 2s poll.
    expect(src).toMatch(/if \(rh === 'error' \|\| rh === 'loading'\)/)
    // Timeout/retry abort in-flight REST via orchestrator (generation + AbortSignal).
    expect(src).toMatch(/signal: opts\?\.signal/)
    expect(src).toMatch(/force: !!opts\?\.force/)
  })

  it('keeps session snapshot cache across hard remount and WS does not clear error', () => {
    expect(src).toMatch(/from '@\/lib\/liveLogSnapshotCache'/)
    expect(src).toMatch(/from '@\/lib\/applyLiveWsAcpPage'/)
    expect(src).toMatch(/restoreEventPagesFromCache/)
    expect(src).toMatch(/clearLiveLogSnapshotsExceptRun/)
    expect(src).toMatch(/syncEventPageToCache/)
    expect(src).toMatch(/selMcpCalls/)
    // Hard remount restores cache after clearing reactive pages.
    expect(src).toMatch(/clearReactiveRecord\(eventPages\)[\s\S]*restoreEventPagesFromCache\(id\)/)
    // Empty WS frames must not rewrite timeline / sync empty cache.
    expect(src).toMatch(/applyLiveWsAcpPage\(eventPages\[m\.nodeId\], wsEvents\)/)
    expect(src).toMatch(/if \(mergedPage\)/)
    // WS merge must not promote rehydrate / clear soft warn.
    expect(src).toMatch(/WS only merges into the snapshot/)
    expect(src).not.toMatch(/rehydrateByNode\[m\.nodeId\]\s*=\s*['"]ready['"]/)
  })
})


describe('RunDetailView desktop review layout budget', () => {
  it('guards review widen to desktop review tab and restores ~520 otherwise', () => {
    expect(src).toMatch(/from '@\/lib\/reviewLayoutBudget'/)
    expect(src).toMatch(/REVIEW_CANVAS_MIN/)
    expect(src).toMatch(/REVIEW_SIDEBAR/)
    expect(src).toMatch(/reviewRightPanelCssWidth/)
    expect(src).toMatch(
      /desktopReviewLayout = computed\(\(\) => !isMobile\.value && nodeTab\.value === 'review'\)/,
    )
    expect(src).toMatch(/:style="reviewRightPanelStyle"/)
    expect(src).toMatch(/:style="canvasPaneStyle"/)
    // Non-review keeps md:w-[520px]; review uses bound budget width instead.
    expect(src).toMatch(/desktopReviewLayout \? '' : 'md:w-\[520px\]'/)
    expect(src).toMatch(/md:w-\[520px\]/)
  })

  it('passes sidebar-width=REVIEW_SIDEBAR only on Run Detail review ReviewShell', () => {
    expect(src).toMatch(
      /nodeTab === 'review'[\s\S]*?<ReviewShell[\s\S]*?:sidebar-width="REVIEW_SIDEBAR"[\s\S]*?:storage-key="REVIEW_SHELL_WIDTH_KEY_REVIEW"/,
    )
    // Default ReviewShell width stays 400 elsewhere — this file must not change the default.
    expect(src).not.toMatch(/sidebarWidth:\s*300/)
  })

  it('binds review right width and canvas min only via desktopReviewLayout styles', () => {
    expect(src).toMatch(
      /reviewRightPanelStyle = computed\(\(\) =>\s*desktopReviewLayout\.value \? \{ width: reviewRightPanelCssWidth\(\) \} : undefined,/,
    )
    expect(src).toMatch(
      /canvasPaneStyle = computed\(\(\) =>\s*desktopReviewLayout\.value \? \{ minWidth: `\$\{REVIEW_CANVAS_MIN\}px` \} : undefined,/,
    )
  })
})

describe('RunDetailView mobile timeline view contract', () => {
  it('does not hide the main-path timeline pane behind hidden md:block', () => {
    expect(src).toMatch(/data-testid="run-timeline-pane"/)
    expect(src).toMatch(/min-h-\[240px\].*run-timeline-pane|data-testid="run-timeline-pane"[\s\S]*?min-h-\[240px]/)
    // Timeline pane itself must not use desktop-only hidden.
    const pane = src.match(/data-testid="run-timeline-pane"[\s\S]{0,200}class="[^"]*"/)
    expect(pane?.[0]).toBeTruthy()
    expect(pane?.[0]).not.toMatch(/\bhidden\b/)
  })

  it('aligns mobile timeline stack min-heights with stats·single', () => {
    expect(src).toMatch(/min-h-\[240px\]/)
    expect(src).toMatch(/min-h-\[320px\] md:min-h-0/)
    // Gate path keeps min-h-0 flex-1 without competing timeline stack min-height.
    expect(src).toMatch(
      /viewMode === 'timeline' &&\s*!\(isMobile && run\.status === 'waiting_human' && nodeTab === 'gate'\)/,
    )
  })

  it('hides canvas entry on mobile and silently normalizes canvas → timeline', () => {
    expect(src).toMatch(/v-if="!isMobile"/)
    expect(src).toMatch(/data-testid="view-mode-canvas"/)
    expect(src).toMatch(/if \(mobile && viewMode\.value === 'canvas'\) viewMode\.value = 'timeline'/)
    expect(src).toMatch(
      /viewMode = ref<'canvas' \| 'timeline' \| 'stats'>\(isMobile\.value \? 'timeline' : 'canvas'\)/,
    )
  })

  it('delegates default node pick to pickDefaultTimelineNodeId', () => {
    expect(src).toMatch(/pickDefaultTimelineNodeId/)
    expect(src).toMatch(/const defaultNode = computed\(\(\) => pickDefaultTimelineNodeId\(run\.value\)\)/)
    // Must not keep the old Object.values().find first-running shortcut alone.
    expect(src).not.toMatch(
      /Object\.values\(run\.value\.nodeRuns\)\s*\n\s*return \(entries\.find/,
    )
  })
})

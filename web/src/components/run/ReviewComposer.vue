<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import ClarifyChat from './ClarifyChat.vue'
import ParagraphInput from '../ui/ParagraphInput.vue'
import GateReactStreamPanel from './GateReactStreamPanel.vue'
import type { ClarifyTurn, ClarifyImage, ReactAnnotation, AcpEvent } from '@/lib/types'
import AnnotationChip from './AnnotationChip.vue'

/**
 * Thin mode wrapper around ClarifyChat / a gate-local composer.
 * - clarify: chips + attachments +「发送澄清回复」(no finish)
 * - review: ClarifyChat chips + attachments + send +「确认并流转」
 * - gate: local composer with the same review semantics —「发送」+「确认并流转」
 *   (no 打回修改 / 通过并流转). Send → GateReactRevise; confirm → ResumeGate(approve/pass).
 */
const props = withDefaults(
  defineProps<{
    mode: 'clarify' | 'review' | 'gate'
    runId?: string
    nodeId?: string
    iteration?: number
    turns?: ClarifyTurn[]
    done?: boolean
    active?: boolean
    /** Gate: hot ReAct send/revise available (unmount send when false / cold). */
    canReject?: boolean
    /** Gate: show confirm action (unmount when false; do not use disabled-only). */
    canPass?: boolean
    /** Gate: send/revise in flight. */
    rejecting?: boolean
    rejectError?: string | null
    /** Gate: cold-session notice (send degraded; confirm remains). */
    coldSession?: boolean
    textOnly?: boolean
    /** Gate: disable confirm (e.g. open PreviewIssues). */
    passDisabled?: boolean
    /**
     * Gate: when true, send may fire without draft/attachments/annotations
     * (e.g. PreviewIssues n_open≥1 — issues already recorded elsewhere).
     */
    rejectAllowEmpty?: boolean
    passTitle?: string
    passLabel?: string
    rejectLabel?: string
    /** Review confirm failure (bottom status bar via ClarifyChat). */
    confirmError?: string | null
    /**
     * Gate sandbox-aligned session UX (same surface as GateApproval mobile-fill /
     * content-fit): pending-send queue, streaming agent text, Cancel.
     */
    queued?: { text: string }[]
    thinking?: boolean
    streamText?: string
    /** ACP thought rail (separate from streamText). */
    streamThought?: string
    interrupted?: boolean
    /** ISO when turn completed normally — drives restrained「已完成」footnote. */
    streamCompletedAt?: string | null
  }>(),
  {
    iteration: 1,
    turns: () => [],
    done: false,
    active: true,
    canReject: true,
    canPass: true,
    rejecting: false,
    rejectError: null,
    coldSession: false,
    textOnly: false,
    passDisabled: false,
    rejectAllowEmpty: false,
    passLabel: '',
    rejectLabel: '',
    confirmError: null,
    queued: () => [],
    thinking: false,
    streamText: '',
    streamThought: '',
    interrupted: false,
    streamCompletedAt: null,
  },
)

const emit = defineEmits<{
  (e: 'send', text: string, images: ClarifyImage[], annotations: ReactAnnotation[]): void
  (e: 'finish'): void
  (e: 'cancel'): void
  /** @deprecated Prefer `send`; kept for GateApproval wiring during review-semantics migrate. */
  (e: 'reject'): void
  /** @deprecated Prefer `finish`; kept for GateApproval wiring during review-semantics migrate. */
  (e: 'pass'): void
}>()

const chatRef = ref<{
  applyReviewFrame: (frame: any) => void
  applyAcpEvents: (events: AcpEvent[] | undefined, nodeId?: string) => boolean | void
  cancelReview: () => void
  discardLastQueued: () => void
} | null>(null)

defineExpose({
  /** Returns false when ClarifyChat is not mounted yet (hard-load race). */
  applyReviewFrame: (frame: any): boolean => {
    if (!chatRef.value?.applyReviewFrame) return false
    chatRef.value.applyReviewFrame(frame)
    return true
  },
  /**
   * Pass through ClarifyChat apply result — false when slot not ready
   * (must not return true merely because chatRef exists).
   */
  applyAcpEvents: (events: AcpEvent[] | undefined, nodeId?: string): boolean => {
    if (!chatRef.value?.applyAcpEvents) return false
    return chatRef.value.applyAcpEvents(events, nodeId) !== false
  },
  cancelReview: () => chatRef.value?.cancelReview(),
  discardLastQueued: () => chatRef.value?.discardLastQueued(),
  isChatReady: () => !!chatRef.value,
})

const { t } = useI18n()

const draft = defineModel<string>('draft', { default: '' })
const attachments = defineModel<ClarifyImage[]>('attachments', { default: () => [] })
const annotations = defineModel<ReactAnnotation[]>('annotations', { default: () => [] })

const canSubmitGate = computed(() => {
  if (props.rejecting) return false
  if (props.rejectAllowEmpty) return true
  return (
    draft.value.trim().length > 0 ||
    attachments.value.length > 0 ||
    annotations.value.length > 0
  )
})

const showGateCancel = computed(
  () => props.thinking || (props.queued?.length ?? 0) > 0,
)

const gateQueued = computed(() => props.queued ?? [])

/**
 * Footer hint must follow open-issue / cold-session state:
 * - rejectAllowEmpty (n_open≥1): open issues already recorded; draft optional for send
 * - canReject (hot send): normal draft threshold
 * - coldSession: send degraded — confirm only
 * - else (confirm-only): submit feedback before send
 */
const gateFooterHint = computed(() => {
  if (props.rejectAllowEmpty) return t('pages.reviewComposer.openIssuesSendHint')
  if (props.canReject) return t('pages.reviewComposer.thresholdHint')
  if (props.coldSession) return t('pages.reviewComposer.coldHint')
  return t('pages.gateApproval.helpReviseDetailNoIssuesNoForm')
})

const sendButtonLabel = computed(
  () => props.rejectLabel || t('pages.reviewComposer.send'),
)
const confirmButtonLabel = computed(
  () => props.passLabel || t('pages.clarify.confirmFlow'),
)

function removeAnnotation(i: number) {
  annotations.value.splice(i, 1)
}

function onSend() {
  if (!canSubmitGate.value || !props.canReject) return
  emit('send', draft.value, attachments.value, annotations.value)
  emit('reject')
}
function onConfirm() {
  if (!props.canPass || props.passDisabled) return
  emit('finish')
  emit('pass')
}
</script>

<template>
  <!-- Clarify / review: reuse ClarifyChat (chip + image + threshold already wired). -->
  <ClarifyChat
    v-if="mode === 'clarify' || mode === 'review'"
    ref="chatRef"
    class="h-full min-h-0"
    :run-id="runId || ''"
    :node-id="nodeId || ''"
    :iteration="iteration"
    v-model:draft="draft"
    v-model:attachments="attachments"
    v-model:annotations="annotations"
    :turns="turns"
    :done="done"
    :active="active"
    :review-mode="mode === 'review'"
    :annotate-enabled="mode === 'clarify' || mode === 'review'"
    :hide-finish="mode === 'clarify'"
    :send-label="mode === 'clarify' ? t('pages.reviewComposer.sendClarify') : undefined"
    :confirm-error="confirmError"
    @send="(text, images, anns) => emit('send', text, images, anns)"
    @finish="emit('finish')"
    @cancel="emit('cancel')"
  />

  <!-- Gate: local composer with review-semantics sticky actions (send + confirm). -->
  <div
    v-else
    class="flex h-full min-h-0 flex-col"
    data-testid="review-composer-gate"
    data-review-composer
  >
    <div
      v-if="coldSession"
      class="shrink-0 border-b border-line bg-warn/10 px-3 py-2 text-[11.5px] leading-relaxed text-warn"
      data-testid="review-composer-cold-note"
    >
      {{ t('pages.reviewComposer.coldNote') }}
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto px-3 py-2 text-[12px] text-txt3">
      <slot name="gate-body">
        {{ t('pages.reviewComposer.gateHint') }}
      </slot>
    </div>
    <div class="shrink-0 border-t border-line p-3" data-testid="review-composer-actions">
      <div v-if="annotations.length" class="mb-2 flex flex-wrap gap-1.5">
        <AnnotationChip
          v-for="(a, ai) in annotations"
          :key="ai"
          :ann="a"
          removable
          test-id="review-annotation-chip"
          @remove="removeAnnotation(ai)"
        />
      </div>
      <ParagraphInput
        v-model:text="draft"
        v-model:images="attachments"
        :text-only="textOnly"
        :disabled="rejecting || !canReject"
        :placeholder="t('pages.gateApproval.reactRevise.placeholder')"
      />
      <div v-if="rejectError" class="mt-1.5 text-[11px] text-err">{{ rejectError }}</div>
      <div class="mt-2 flex flex-wrap gap-2">
        <button
          v-if="canReject"
          type="button"
          class="inline-flex flex-1 items-center justify-center gap-1.5 bg-accent/15 px-3 py-2 text-sm font-medium text-accent-2 transition hover:bg-accent/25 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="review-composer-send"
          :disabled="!canSubmitGate"
          @click="onSend"
        >
          <Icon name="arrow-left" :size="14" />
          {{ rejecting ? t('pages.gateApproval.reactRevise.sending') : sendButtonLabel }}
        </button>
        <button
          v-if="canPass"
          type="button"
          class="inline-flex flex-1 items-center justify-center gap-1.5 bg-ok/15 px-3 py-2 text-sm font-medium text-ok transition hover:bg-ok/25 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="review-composer-pass"
          :disabled="passDisabled"
          :title="passTitle"
          @click="onConfirm"
        >
          <Icon name="check" :size="14" />
          {{ confirmButtonLabel }}
        </button>
        <button
          v-if="showGateCancel"
          type="button"
          class="inline-flex items-center justify-center gap-1.5 border border-line bg-elevated px-3 py-2 text-sm font-medium text-txt2"
          data-testid="gate-react-cancel"
          title="Cancel"
          @click="emit('cancel')"
        >
          Cancel
        </button>
      </div>
      <div
        v-if="gateQueued.length"
        class="mt-2 rounded border border-line bg-base/40 px-2 py-1.5"
        data-testid="gate-react-queue"
      >
        <div class="mb-1 text-[11px] text-txt3">
          {{ t('pages.agentChatTester.queue', { n: gateQueued.length }) }}
        </div>
        <div
          v-for="(q, qi) in gateQueued"
          :key="qi"
          class="truncate text-[12px] text-txt2"
        >
          {{ qi + 1 }}. {{ q.text }}
        </div>
      </div>
      <GateReactStreamPanel
        :thinking="thinking"
        :stream-text="streamText"
        :stream-thought="streamThought"
        :interrupted="interrupted"
        :completed-at="streamCompletedAt"
      />
      <p class="mt-2 text-[11px] leading-relaxed text-txt3" data-testid="review-composer-footer-hint">
        {{ gateFooterHint }}
      </p>
    </div>
  </div>
</template>

<style scoped>
/* typing-dots / outputting / caret live in GateReactStreamPanel */
</style>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import ClarifyChat from './ClarifyChat.vue'
import ParagraphInput from '../ui/ParagraphInput.vue'
import type { ClarifyTurn, ClarifyImage, ReactAnnotation, AcpEvent } from '@/lib/types'
import AnnotationChip from './AnnotationChip.vue'

/**
 * Thin mode wrapper around ClarifyChat / a gate-local composer.
 * - clarify: chips + attachments +「发送澄清回复」(no finish / pass / reject)
 * - review: chips + attachments + send +「确认并流转」
 * - gate: chips + attachments +「打回修改」/「通过并流转」
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
    /** Gate: hot ReAct revise available (unmount reject when false). */
    canReject?: boolean
    /** Gate: show pass action (unmount pass when false; do not use disabled-only). */
    canPass?: boolean
    /** Gate: reject in flight. */
    rejecting?: boolean
    rejectError?: string | null
    /** Gate: cold-session notice. */
    coldSession?: boolean
    textOnly?: boolean
    /** Gate: disable pass (e.g. open PreviewIssues). */
    passDisabled?: boolean
    /**
     * Gate: when true, reject may fire without draft/attachments/annotations
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
  },
)

const emit = defineEmits<{
  (e: 'send', text: string, images: ClarifyImage[], annotations: ReactAnnotation[]): void
  (e: 'finish'): void
  (e: 'cancel'): void
  (e: 'reject'): void
  (e: 'pass'): void
}>()

const chatRef = ref<{
  applyReviewFrame: (frame: any) => void
  applyAcpEvents: (events: AcpEvent[] | undefined, nodeId?: string) => void
  cancelReview: () => void
  discardLastQueued: () => void
} | null>(null)

defineExpose({
  applyReviewFrame: (frame: any) => chatRef.value?.applyReviewFrame(frame),
  applyAcpEvents: (events: AcpEvent[] | undefined, nodeId?: string) =>
    chatRef.value?.applyAcpEvents(events, nodeId),
  cancelReview: () => chatRef.value?.cancelReview(),
  discardLastQueued: () => chatRef.value?.discardLastQueued(),
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
 * Footer hint must follow n_open mutex / cold-session state:
 * - rejectAllowEmpty (n_open≥1): open issues already recorded; draft optional
 * - canReject: normal draft threshold
 * - coldSession: hot reject degraded to cold fail/revise
 * - else (pass-only, e.g. PreviewIssues n_open=0): submit feedback before reject
 */
const gateFooterHint = computed(() => {
  if (props.rejectAllowEmpty) return t('pages.reviewComposer.openIssuesRejectHint')
  if (props.canReject) return t('pages.reviewComposer.thresholdHint')
  if (props.coldSession) return t('pages.reviewComposer.coldHint')
  return t('pages.gateApproval.helpReviseDetailNoIssuesNoForm')
})

function removeAnnotation(i: number) {
  annotations.value.splice(i, 1)
}

function onReject() {
  if (!canSubmitGate.value || !props.canReject) return
  emit('reject')
}
function onPass() {
  if (!props.canPass || props.passDisabled) return
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

  <!-- Gate: local composer with dual sticky actions (no ClarifyChat turns required). -->
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
        :disabled="rejecting"
        :placeholder="t('pages.gateApproval.reactRevise.placeholder')"
      />
      <div v-if="rejectError" class="mt-1.5 text-[11px] text-err">{{ rejectError }}</div>
      <div class="mt-2 flex flex-wrap gap-2">
        <button
          v-if="canReject"
          type="button"
          class="inline-flex flex-1 items-center justify-center gap-1.5 bg-warn/15 px-3 py-2 text-sm font-medium text-warn transition hover:bg-warn/25 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="review-composer-reject"
          :disabled="!canSubmitGate"
          @click="onReject"
        >
          <Icon name="arrow-left" :size="14" />
          {{ rejecting ? t('pages.gateApproval.reactRevise.sending') : rejectLabel || t('pages.reviewComposer.reject') }}
        </button>
        <button
          v-if="canPass"
          type="button"
          class="inline-flex flex-1 items-center justify-center gap-1.5 bg-ok/15 px-3 py-2 text-sm font-medium text-ok transition hover:bg-ok/25 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="review-composer-pass"
          :disabled="passDisabled"
          :title="passTitle"
          @click="onPass"
        >
          <Icon name="check" :size="14" />
          {{ passLabel || t('pages.reviewComposer.pass') }}
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
      <div
        v-if="thinking"
        class="mt-2 max-h-40 space-y-1.5 overflow-y-auto rounded border border-line bg-surface px-2 py-1.5 text-[12px] text-txt2"
        data-testid="gate-react-stream"
      >
        <div
          v-if="!streamText && !streamThought"
          class="inline-flex items-center gap-2 text-txt3"
          data-testid="gate-busy-placeholder"
        >
          <span class="typing-dots" aria-hidden="true"><i /><i /><i /></span>
          <span>{{ t('pages.clarify.thinkingBusy') }}</span>
        </div>
        <div
          v-else-if="!streamText && streamThought"
          class="text-txt3"
          data-testid="gate-busy-status"
        >
          {{ t('pages.clarify.thinkingBusy') }}
        </div>
        <div
          v-else-if="streamText"
          class="composer-outputting"
          data-testid="gate-busy-status"
        >
          {{ t('pages.clarify.outputting') }}
        </div>
        <details
          v-if="streamThought"
          open
          class="rounded border border-line bg-base/60 text-[11.5px] text-txt3"
          data-testid="gate-react-thought"
        >
          <summary class="flex cursor-pointer select-none items-center gap-1 px-2 py-1 hover:text-txt2">
            <Icon name="sparkles" :size="11" class="text-accent-2" />
            {{ t('pages.clarify.thought') }}
          </summary>
          <div class="whitespace-pre-wrap border-t border-dashed border-line px-2 pb-1.5 pt-1 font-mono leading-5">{{ streamThought }}</div>
        </details>
        <div v-if="streamText">
          {{ streamText }}
          <span v-if="interrupted" class="ml-1 text-[10px] text-warn">interrupted</span>
        </div>
      </div>
      <p class="mt-2 text-[11px] leading-relaxed text-txt3" data-testid="review-composer-footer-hint">
        {{ gateFooterHint }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.typing-dots {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.typing-dots i {
  width: 5px;
  height: 5px;
  border-radius: 9999px;
  background: #22d3ee;
  animation: typing-bounce 1.2s infinite ease-in-out both;
}
.typing-dots i:nth-child(2) {
  animation-delay: 0.16s;
}
.typing-dots i:nth-child(3) {
  animation-delay: 0.32s;
}
@keyframes typing-bounce {
  0%,
  70%,
  100% {
    transform: translateY(0);
    opacity: 0.35;
  }
  35% {
    transform: translateY(-4px);
    opacity: 1;
  }
}
.composer-outputting {
  color: rgb(var(--c-accent-2));
  background: var(--grad-logo);
  background-size: var(--grad-logo-size);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  animation: shimmer 3.5s ease-in-out infinite;
}
</style>

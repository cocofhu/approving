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
  applyAcpEvents: (events: AcpEvent[] | undefined) => void
  cancelReview: () => void
} | null>(null)

defineExpose({
  applyReviewFrame: (frame: any) => chatRef.value?.applyReviewFrame(frame),
  applyAcpEvents: (events: AcpEvent[] | undefined) => chatRef.value?.applyAcpEvents(events),
  cancelReview: () => chatRef.value?.cancelReview(),
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
      <div class="mt-2 flex gap-2">
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
      </div>
      <p class="mt-2 text-[11px] leading-relaxed text-txt3" data-testid="review-composer-footer-hint">
        {{ gateFooterHint }}
      </p>
    </div>
  </div>
</template>

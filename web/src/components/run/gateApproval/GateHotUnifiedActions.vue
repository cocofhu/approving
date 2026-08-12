<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGateApprovalCtx } from './gateApprovalContext'
import Icon from '../../ui/Icon.vue'
import GateReactStreamPanel from '../GateReactStreamPanel.vue'

const props = defineProps<{
  /** mobile: always touch-sized; content-fit: min-height only when isMobile */
  layout: 'mobile' | 'content-fit'
}>()

const { t } = useI18n()
const { s } = useGateApprovalCtx()

const recordBtnClass = computed(() => {
  if (props.layout === 'mobile') {
    return 'inline-flex min-h-[36px] items-center justify-center rounded-md border border-line px-3 text-xs font-medium text-txt2 transition hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50'
  }
  return [
    'inline-flex items-center justify-center rounded-md border border-line px-3 py-1.5 text-xs font-medium text-txt2 transition hover:bg-elevated disabled:cursor-not-allowed disabled:opacity-50',
    s.isMobile ? 'min-h-[36px]' : '',
  ]
})

const actionBtnBase = computed(() => {
  if (props.layout === 'mobile') {
    return 'inline-flex min-h-[44px] flex-1 items-center justify-center gap-1.5 px-3 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50'
  }
  return [
    'inline-flex flex-1 items-center justify-center gap-1.5 px-3 py-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50',
    s.isMobile ? 'min-h-[44px]' : '',
  ]
})

const cancelBtnClass = computed(() => {
  if (props.layout === 'mobile') {
    return 'inline-flex min-h-[44px] items-center justify-center gap-1.5 border border-line bg-elevated px-3 text-sm font-medium text-txt2'
  }
  return [
    'inline-flex items-center justify-center gap-1.5 border border-line bg-elevated px-3 py-2 text-sm font-medium text-txt2',
    s.isMobile ? 'min-h-[44px]' : '',
  ]
})
</script>

<template>
  <div v-if="s.reactError" class="mb-1.5 text-[11px] text-err">{{ s.reactError }}</div>
  <!-- review semantics: 记入 + 发送 + 确认并流转 -->
  <div class="mb-2 flex justify-end">
    <button
      type="button"
      :class="recordBtnClass"
      data-testid="review-record-issue"
      :disabled="!s.canRecordIssue"
      @click="s.recordFeedbackIssue"
    >
      {{ t('pages.gateApproval.reviewFeedback.record') }}
    </button>
  </div>
  <div class="flex flex-wrap gap-2" data-testid="review-composer-actions">
    <button
      v-if="s.showHotReject"
      type="button"
      class="bg-accent/15 text-accent-2 hover:bg-accent/25"
      :class="actionBtnBase"
      data-testid="review-composer-send"
      :disabled="s.reactSending || (!s.hotRejectAllowEmpty && !s.canSubmitReact)"
      @click="s.sendHotReject"
    >
      <Icon name="arrow-left" :size="14" />
      {{ s.reactSending ? t('pages.gateApproval.reactRevise.sending') : s.composerRejectLabel }}
    </button>
    <button
      v-if="s.showHotPass"
      type="button"
      class="bg-ok/15 text-ok hover:bg-ok/25"
      :class="actionBtnBase"
      data-testid="review-composer-pass"
      :disabled="s.composerPassDisabled"
      :title="s.passAction ? s.actionButtonTitle(s.passAction.id) : ''"
      @click="s.onComposerPass"
    >
      <Icon name="check" :size="14" />
      {{ t('pages.clarify.confirmFlow') }}
    </button>
    <button
      v-if="s.canReactRevise && (s.reactThinking || s.reactQueued.length)"
      type="button"
      :class="cancelBtnClass"
      data-testid="gate-react-cancel"
      title="Cancel"
      @click="s.cancelReactRevise"
    >
      Cancel
    </button>
  </div>
  <div
    v-if="s.reactQueued.length"
    class="mt-2 rounded border border-line bg-base/40 px-2 py-1.5"
    data-testid="gate-react-queue"
  >
    <div class="mb-1 text-[11px] text-txt3">
      {{ t('pages.agentChatTester.queue', { n: s.reactQueued.length }) }}
    </div>
    <div
      v-for="(q, qi) in s.reactQueued"
      :key="qi"
      class="truncate text-[12px] text-txt2"
    >
      {{ qi + 1 }}. {{ q.text }}
    </div>
  </div>
  <GateReactStreamPanel
    :thinking="s.reactThinking"
    :stream-text="s.reactStreamText"
    :stream-thought="s.reactStreamThought"
    :interrupted="s.reactInterrupted"
    :completed-at="s.reactStreamCompletedAt"
  />
</template>

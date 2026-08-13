<script lang="ts">
export type FeedbackAttachment = {
  ref?: string
  name?: string
  mimeType?: string
  sizeBytes?: number
}

export type FeedbackAnnotation = {
  jsonPath?: string
  selector?: string
  url?: string
  label?: string
  note?: string
  quote?: string
  truncated?: boolean
}

export type FeedbackTurn = {
  role?: string
  text?: string
  at?: string
  annotations?: FeedbackAnnotation[]
  attachments?: FeedbackAttachment[]
  interrupted?: boolean
}

export type FeedbackTarget = {
  name?: string
  before?: string
  after?: string
  changed?: boolean
}

export type FeedbackRoundDoc = {
  runId?: string
  kind?: string
  node?: { id?: string; label?: string; type?: string }
  iteration?: number
  round?: number
  seq?: number
  at?: string
  actor?: { name?: string; callerKind?: string; unattributable?: boolean }
  action?: string
  interrupted?: boolean
  feedback?: {
    text?: string
    annotations?: FeedbackAnnotation[]
    attachments?: FeedbackAttachment[]
  }
  transcript?: FeedbackTurn[]
  targets?: FeedbackTarget[]
  priorRounds?: { round?: number; kind?: string; at?: string; summary?: string }[]
  prev?: string
  index?: string
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AnnotationChip from '../AnnotationChip.vue'
import ChatImageThumb from '../../ui/ChatImageThumb.vue'
import { blobContentUrl } from '@/lib/api/api'
import { fmtTime } from '@/lib/shared/format'
import type { ReactAnnotation } from '@/lib/shared/types'

const props = defineProps<{ doc: FeedbackRoundDoc }>()

const { t } = useI18n()

const body = computed(() => (props.doc.feedback?.text || '').trim())
const annotations = computed(() => props.doc.feedback?.annotations || [])
const attachments = computed(() => props.doc.feedback?.attachments || [])
const transcript = computed(() => props.doc.transcript || [])
/** Only changed products are worth showing: unchanged rows are noise. */
const changedTargets = computed(() => (props.doc.targets || []).filter((x) => x.changed))

function attachmentSrc(a: FeedbackAttachment): string {
  return a.ref ? blobContentUrl(a.ref) : ''
}

function attachmentLabel(a: FeedbackAttachment, i: number): string {
  return a.name || `#${i + 1}`
}

/** AnnotationChip takes the platform ReactAnnotation shape; the JSON matches it. */
function asAnnotation(a: FeedbackAnnotation): ReactAnnotation {
  return a as ReactAnnotation
}
</script>

<template>
  <div class="space-y-3">
    <p v-if="body" class="whitespace-pre-wrap text-[12px] leading-relaxed text-txt">{{ body }}</p>

    <div v-if="annotations.length" class="flex flex-wrap gap-1.5">
      <AnnotationChip v-for="(a, i) in annotations" :key="i" :ann="asAnnotation(a)" />
    </div>

    <section v-if="attachments.length">
      <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t('pages.product.feedback.attachments', { n: attachments.length }) }}
      </div>
      <div class="flex flex-wrap gap-1.5">
        <ChatImageThumb
          v-for="(a, i) in attachments"
          :key="i"
          mode="previewable"
          :src="attachmentSrc(a)"
          :label="attachmentLabel(a, i)"
        />
      </div>
    </section>

    <section v-if="changedTargets.length">
      <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t('pages.product.feedback.targets') }}
      </div>
      <ul class="space-y-1">
        <li v-for="(x, i) in changedTargets" :key="i" class="flex flex-wrap items-center gap-1.5 text-[11px] text-txt2">
          <code class="font-mono text-[12px] text-txt">{{ x.name }}</code>
          <span class="text-txt3">{{ x.before || '—' }} → {{ x.after || '—' }}</span>
        </li>
      </ul>
    </section>

    <section v-if="transcript.length">
      <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t('pages.product.feedback.transcript') }}
      </div>
      <div class="space-y-1.5">
        <div
          v-for="(turn, i) in transcript"
          :key="i"
          class="rounded-md border p-2"
          :class="turn.role === 'human' ? 'border-accent/40 bg-accent-dim/25' : 'border-line bg-base/40'"
        >
          <div class="mb-0.5 flex items-center gap-2">
            <span class="text-[10px] font-medium uppercase tracking-wide" :class="turn.role === 'human' ? 'text-accent-2' : 'text-txt3'">
              {{ turn.role === 'human' ? t('pages.product.feedback.roleHuman') : t('pages.product.feedback.roleAgent') }}
            </span>
            <span v-if="turn.interrupted" class="text-[10px] text-warn">{{ t('pages.product.feedback.interrupted') }}</span>
            <span v-if="turn.at" class="ml-auto text-[10px] text-txt3">{{ fmtTime(turn.at) }}</span>
          </div>
          <p class="whitespace-pre-wrap text-[11px] leading-relaxed text-txt2">{{ turn.text }}</p>
        </div>
      </div>
    </section>

    <section v-if="doc.priorRounds?.length">
      <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-txt3">
        {{ t('pages.product.feedback.priorRounds') }}
      </div>
      <ul class="space-y-1">
        <li v-for="(p, i) in doc.priorRounds" :key="i" class="flex items-start gap-1.5 text-[11px] leading-5 text-txt3">
          <span class="shrink-0 text-txt3">{{ t('pages.product.feedback.roundN', { n: p.round }) }}</span>
          <span class="min-w-0 flex-1">{{ p.summary }}</span>
        </li>
      </ul>
    </section>
  </div>
</template>

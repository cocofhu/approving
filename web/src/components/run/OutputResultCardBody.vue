<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/lib/shared/markdown'
import { isVisualHtmlCard } from '@/lib/run/isVisualHtmlCard'
import StructuredArtifactView from './StructuredArtifactView.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'
import type { OutputCard, Run } from '@/lib/shared/types'

const props = defineProps<{
  card: OutputCard
  run: Run
  doc: unknown
  loading: boolean
  artifactHtml: string
  /** True when artifact fetch failed (network/404). Empty content without error is separate. */
  artifactLoadError?: boolean
  /** detail: narrow pane (fitContent HTML). enlarge: card-level modal (fixed viewport). */
  variant: 'detail' | 'enlarge'
}>()

const { t } = useI18n()

const isVisualHtml = computed(() =>
  isVisualHtmlCard(props.card, {
    artifactHtml: props.artifactHtml,
    parsedDoc: props.doc,
  }),
)

/** Prefer fetched artifact body; fall back to inline markdown (HTML from node output). */
const htmlBody = computed(() => {
  const fromArt = props.artifactHtml?.trim() ?? ''
  if (fromArt) return fromArt
  return props.card.markdown?.trim() ?? ''
})

/**
 * Visual HTML preview states — Demo / clarification f5:
 * load-error vs empty body must stay distinct; never fall through to renderMarkdown.
 */
const htmlPreviewState = computed<'loading' | 'load-error' | 'empty' | 'ready'>(() => {
  if (!isVisualHtml.value) return 'ready'
  if (props.loading && !htmlBody.value) return 'loading'
  if (htmlBody.value) return 'ready'
  if (props.artifactLoadError) return 'load-error'
  return 'empty'
})
</script>

<template>
  <template v-if="card.status === 'failed'">
    <p class="text-[12px] text-txt2">
      <strong class="text-err">{{ t('pages.nodeOutput.outputCards.sourceFailedTitle') }}</strong><br />
      {{ card.errorReason || t('pages.nodeOutput.outputCards.invalidSource') }}
    </p>
  </template>
  <template v-else-if="isVisualHtml">
    <div
      v-if="htmlPreviewState === 'loading'"
      class="flex min-h-[160px] flex-col items-center justify-center gap-2 px-4 py-8 text-center"
      data-testid="output-result-html-loading"
    >
      <p class="text-[12px] text-txt3">…</p>
    </div>
    <div
      v-else-if="htmlPreviewState === 'load-error'"
      class="flex min-h-[160px] flex-col items-center justify-center gap-2 px-4 py-8 text-center"
      data-testid="output-result-html-load-error"
      role="alert"
    >
      <p class="text-[10px] font-bold uppercase tracking-wider text-txt3">
        {{ t('pages.nodeOutput.outputCards.previewLoadErrorLabel') }}
      </p>
      <p class="text-[13px] font-medium text-txt">
        {{ t('pages.nodeOutput.outputCards.previewLoadErrorTitle') }}
      </p>
      <p class="max-w-[36ch] text-[12px] text-txt3">
        {{ t('pages.nodeOutput.outputCards.previewLoadErrorBody') }}
      </p>
    </div>
    <div
      v-else-if="htmlPreviewState === 'empty'"
      class="flex min-h-[160px] flex-col items-center justify-center gap-2 px-4 py-8 text-center"
      data-testid="output-result-html-empty"
    >
      <p class="text-[10px] font-bold uppercase tracking-wider text-txt3">
        {{ t('pages.nodeOutput.outputCards.previewEmptyLabel') }}
      </p>
      <p class="text-[13px] font-medium text-txt">
        {{ t('pages.nodeOutput.outputCards.previewEmptyTitle') }}
      </p>
      <p class="max-w-[36ch] text-[12px] text-txt3">
        {{ t('pages.nodeOutput.outputCards.previewEmptyBody') }}
      </p>
    </div>
    <template v-else>
      <!-- Enlarge: determinate 70vh so iframe h-full works (F4 / review v1). Detail: fitContent, scroll with right pane (F7). -->
      <div
        v-if="variant === 'enlarge'"
        class="h-[70vh] min-h-0"
        data-testid="output-result-enlarge-html-viewport"
      >
        <HtmlPreview :html="htmlBody" :enlargeable="false" />
      </div>
      <HtmlPreview
        v-else
        :html="htmlBody"
        :enlargeable="false"
        :fit-content="true"
      />
    </template>
  </template>
  <template v-else-if="card.typeTag === '结构化产物' && card.structuredArtifactName && doc">
    <StructuredArtifactView
      :name="card.structuredArtifactName"
      :doc="doc"
      :artifacts="run.artifacts"
      :run-id="run.id"
      :run-status="run.status"
    />
  </template>
  <template v-else-if="card.typeTag === '结构化产物' && card.markdown">
    <div class="md text-[12px] leading-relaxed text-txt2" v-html="renderMarkdown(card.markdown)" />
  </template>
  <template v-else-if="card.typeTag === '自定义产物' && card.artifactName">
    <div v-if="loading && !htmlBody" class="text-[12px] text-txt3">…</div>
    <pre
      v-else
      class="whitespace-pre-wrap border border-line bg-base p-2.5 font-mono text-[11px] leading-relaxed text-txt2"
      :class="variant === 'detail' ? 'scroll-area max-h-48 overflow-y-auto' : ''"
    >{{ artifactHtml }}</pre>
  </template>
  <template v-else-if="card.typeTag === 'Markdown' && card.markdown">
    <div class="md text-[12px] leading-relaxed text-txt2" v-html="renderMarkdown(card.markdown)" />
  </template>
  <template v-else>
    <p class="text-[12px] text-txt3">{{ t('pages.nodeOutput.outputCards.invalidSource') }}</p>
  </template>
</template>

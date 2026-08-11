<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/lib/shared/markdown'
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

function isHtmlArtifact(name?: string): boolean {
  return !!name && name.endsWith('.html')
}

const isCustomHtml = computed(
  () => props.card.typeTag === '自定义产物' && isHtmlArtifact(props.card.artifactName),
)

/**
 * Prefer the card's outputs.page snapshot for visual pages so two cards never
 * share a stale global page.html fetch. Otherwise use the artifact loaded by
 * artifactName + nodeId, then markdown.
 */
const htmlBody = computed(() => {
  const fromMd = props.card.markdown?.trim() ?? ''
  if (fromMd && props.card.outputKey === 'page') return fromMd
  const fromArt = props.artifactHtml?.trim() ?? ''
  if (fromArt) return fromArt
  return fromMd
})

/**
 * Custom HTML preview state — align with GateProductEditor empty/error:
 * never fall through to renderMarkdown for *.html custom cards.
 */
const htmlPreviewState = computed<'loading' | 'unavailable' | 'ready'>(() => {
  if (!isCustomHtml.value) return 'ready'
  if (props.loading && !htmlBody.value) return 'loading'
  if (!htmlBody.value) return 'unavailable'
  if (props.artifactLoadError && !props.artifactHtml?.trim() && !props.card.markdown?.trim()) {
    return 'unavailable'
  }
  return 'ready'
})
</script>

<template>
  <template v-if="card.status === 'failed'">
    <p class="text-[12px] text-txt2">
      <strong class="text-err" data-testid="output-result-fail-title">{{
        card.failTitle || t('pages.nodeOutput.outputCards.sourceFailedTitle')
      }}</strong><br />
      {{ card.errorReason || t('pages.nodeOutput.outputCards.invalidSource') }}
    </p>
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
    <div v-if="loading && !htmlBody && !isCustomHtml" class="text-[12px] text-txt3">…</div>
    <template v-else-if="isHtmlArtifact(card.artifactName)">
      <div
        v-if="htmlPreviewState === 'loading'"
        class="flex min-h-[160px] flex-col items-center justify-center gap-2 px-4 py-8 text-center"
        data-testid="output-result-html-loading"
      >
        <p class="text-[12px] text-txt3">…</p>
      </div>
      <div
        v-else-if="htmlPreviewState === 'unavailable'"
        class="flex min-h-[160px] flex-col items-center justify-center gap-2 px-4 py-8 text-center"
        data-testid="output-result-html-unavailable"
        role="alert"
      >
        <p class="text-[13px] font-medium text-txt">
          {{ t('pages.nodeOutput.outputCards.previewUnavailableTitle') }}
        </p>
        <p class="max-w-[36ch] text-[12px] text-txt3">
          {{ t('pages.nodeOutput.outputCards.previewUnavailableBody') }}
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

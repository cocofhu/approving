<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/lib/markdown'
import StructuredArtifactView from './StructuredArtifactView.vue'
import HtmlPreview from '../ui/HtmlPreview.vue'
import type { OutputCard, Run } from '@/lib/types'

const props = defineProps<{
  card: OutputCard
  run: Run
  doc: unknown
  loading: boolean
  artifactHtml: string
  /** detail: narrow pane (fitContent HTML). enlarge: card-level modal (fixed viewport). */
  variant: 'detail' | 'enlarge'
}>()

const { t } = useI18n()

function isHtmlArtifact(name?: string): boolean {
  return !!name && name.endsWith('.html')
}
</script>

<template>
  <template v-if="card.status === 'failed'">
    <p class="text-[12px] text-txt2">
      <strong class="text-err">{{ t('pages.nodeOutput.outputCards.sourceFailedTitle') }}</strong><br />
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
    <div v-if="loading && !artifactHtml" class="text-[12px] text-txt3">…</div>
    <template v-else-if="isHtmlArtifact(card.artifactName)">
      <!-- Enlarge: determinate 70vh so iframe h-full works (F4 / review v1). Detail: fitContent, scroll with right pane (F7). -->
      <div
        v-if="variant === 'enlarge'"
        class="h-[70vh] min-h-0"
        data-testid="output-result-enlarge-html-viewport"
      >
        <HtmlPreview :html="artifactHtml || card.markdown || ''" :enlargeable="false" />
      </div>
      <HtmlPreview
        v-else
        :html="artifactHtml || card.markdown || ''"
        :enlargeable="false"
        :fit-content="true"
      />
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

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AnnotateBtn from './AnnotateBtn.vue'

export type ReviewFinding = {
  id?: string
  severity?: string
  file?: string
  line?: number
  title?: string
  detail?: string
  suggestion?: string
}
export type ReviewDoc = {
  summary?: string
  verdict?: string
  findings?: ReviewFinding[]
  action_items?: string[]
}

const props = defineProps<{ doc: ReviewDoc }>()

const { t } = useI18n()

const VERDICT: Record<string, { labelKey: string; cls: string; icon: string }> = {
  approve: { labelKey: 'pages.product.review.verdictLabels.approve', cls: 'bg-ok/15 text-ok border-ok/40', icon: '✓' },
  approve_with_comments: { labelKey: 'pages.product.review.verdictLabels.approve_with_comments', cls: 'bg-info/15 text-info border-info/40', icon: '✓' },
  request_changes: { labelKey: 'pages.product.review.verdictLabels.request_changes', cls: 'bg-warn/15 text-warn border-warn/40', icon: '↻' },
  reject: { labelKey: 'pages.product.review.verdictLabels.reject', cls: 'bg-err/15 text-err border-err/40', icon: '✕' },
}
const verdict = computed(() => {
  const v = VERDICT[props.doc.verdict || '']
  if (v) return { label: t(v.labelKey), cls: v.cls, icon: v.icon }
  return { label: props.doc.verdict || '—', cls: 'bg-base text-txt3 border-line', icon: '?' }
})

const SEV: Record<string, { label: string; cls: string }> = {
  critical: { label: 'CRITICAL', cls: 'bg-err/20 text-err' },
  high: { label: 'HIGH', cls: 'bg-err/15 text-err' },
  medium: { label: 'MEDIUM', cls: 'bg-warn/15 text-warn' },
  low: { label: 'LOW', cls: 'bg-info/15 text-info' },
}
function sev(s?: string) {
  return SEV[s || 'medium'] || SEV.medium
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center gap-2.5 rounded-lg border p-3" :class="verdict.cls">
      <span class="flex h-8 w-8 items-center justify-center rounded-full bg-white/10 text-[15px] font-bold">{{ verdict.icon }}</span>
      <div class="min-w-0 flex-1">
        <div class="text-[10px] uppercase tracking-wider opacity-70">{{ t('pages.product.review.verdict') }}</div>
        <div class="text-[14px] font-semibold">{{ verdict.label }}</div>
      </div>
    </div>

    <div v-if="doc.summary" class="group flex items-start gap-1 rounded-lg border border-line bg-base/40 p-3 text-[12px] leading-relaxed text-txt2">
      <span class="min-w-0 flex-1" data-json-path="summary" data-label="概述">{{ doc.summary }}</span>
      <AnnotateBtn json-path="summary" label="概述" />
    </div>

    <section v-if="doc.findings?.length">
      <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.product.review.findings', { n: doc.findings.length }) }}</div>
      <div class="space-y-2">
        <div v-for="(f, i) in doc.findings" :key="f.id || i" class="group rounded-lg border border-line bg-base/40 p-2.5">
          <div class="flex flex-wrap items-center gap-2">
            <span class="shrink-0 rounded px-1.5 py-0.5 text-[9px] font-bold tracking-wide" :class="sev(f.severity).cls">{{ sev(f.severity).label }}</span>
            <span
              class="text-[12px] font-medium text-txt"
              :data-json-path="`findings[${f.id || i}]`"
              :data-label="f.title || `意见 ${i + 1}`"
            >{{ f.title }}</span>
            <AnnotateBtn :json-path="`findings[${f.id || i}]`" :label="f.title || `意见 ${i + 1}`" />
            <code v-if="f.file" class="ml-auto shrink-0 truncate font-mono text-[10px] text-txt3">{{ f.file }}<span v-if="f.line">:{{ f.line }}</span></code>
          </div>
          <div
            v-if="f.detail"
            class="mt-1 text-[11px] leading-relaxed text-txt3"
            :data-json-path="`findings[${f.id || i}].detail`"
            :data-label="`${f.id || i} 详情`"
          >{{ f.detail }}</div>
          <div
            v-if="f.suggestion"
            class="mt-1.5 flex items-start gap-1.5 rounded-md bg-accent-dim/30 px-2 py-1 text-[11px] leading-5 text-txt2"
            :data-json-path="`findings[${f.id || i}].suggestion`"
            :data-label="`${f.id || i} 建议`"
          >
            <span class="mt-0.5 shrink-0 text-accent-2">{{ t('pages.product.review.suggestion') }}</span>{{ f.suggestion }}
          </div>
        </div>
      </div>
    </section>

    <section v-if="doc.action_items?.length">
      <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-txt3">{{ t('pages.product.review.actionItems') }}</div>
      <ul class="space-y-1">
        <li v-for="(a, i) in doc.action_items" :key="i" class="group flex items-start gap-1.5 text-[11px] leading-5 text-txt2">
          <span class="mt-0.5 shrink-0 text-warn">☐</span>
          <span class="min-w-0 flex-1" :data-json-path="`action_items[${i}]`" :data-label="a">{{ a }}</span>
          <AnnotateBtn :json-path="`action_items[${i}]`" :label="a" />
        </li>
      </ul>
    </section>
  </div>
</template>

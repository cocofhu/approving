<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import type { ProgressCitation } from '@/lib/shared/types'
import {
  isBareExtractSnippet,
  isValidPmCitationShape,
  shortRunId,
} from '@/lib/pm/pmCitationShape'

const props = defineProps<{ citation: ProgressCitation }>()
const { t } = useI18n()
const router = useRouter()
const expanded = ref(false)

const shapeValid = computed(() =>
  isValidPmCitationShape(props.citation.type, props.citation.targetId),
)

const mainLabel = computed(() => {
  const c = props.citation
  if (!shapeValid.value) {
    return c.summarySnippet || `${c.type}:${c.targetId}`
  }
  switch (c.type) {
    case 'run':
      return `Run #${shortRunId(c.targetId)}`
    case 'workflow':
      return c.summarySnippet && !isBareExtractSnippet(c.type, c.summarySnippet, c.targetId)
        ? c.summarySnippet
        : `Workflow #${c.targetId.replace(/^wf-/i, '')}`
    case 'artifact':
      return c.summarySnippet && !isBareExtractSnippet(c.type, c.summarySnippet, c.targetId)
        ? c.summarySnippet
        : c.targetId
    case 'gate':
      return c.summarySnippet && !isBareExtractSnippet(c.type, c.summarySnippet, c.targetId)
        ? c.summarySnippet
        : `Gate · ${c.targetId}`
    case 'plan':
      return c.summarySnippet && !isBareExtractSnippet(c.type, c.summarySnippet, c.targetId)
        ? c.summarySnippet
        : `Plan ${c.targetId}`
    default:
      return c.summarySnippet || `${c.type}: ${c.targetId}`
  }
})

const subLabel = computed(() => {
  const c = props.citation
  if (!shapeValid.value) return ''
  if (c.type === 'run') {
    const sn = c.summarySnippet?.trim()
    if (!sn || isBareExtractSnippet(c.type, sn, c.targetId)) return ''
    return sn
  }
  return ''
})

function hrefFor(): string | null {
  if (!shapeValid.value) return null
  const c = props.citation
  switch (c.type) {
    case 'run':
      return `/runs/${c.targetId}`
    case 'gate': {
      const runId = c.targetId.includes(':') ? c.targetId.split(':')[0] : c.targetId
      return `/runs/${runId}`
    }
    case 'artifact':
      return `/artifacts?q=${encodeURIComponent(c.targetId)}`
    case 'workflow':
      return `/workflows/${c.targetId}`
    case 'plan': {
      if (c.targetId.includes(':')) {
        const [runId] = c.targetId.split(':')
        return `/runs/${runId}`
      }
      // Bare plan id has no stable run route; treat as non-navigable.
      return null
    }
    default:
      return null
  }
}

const canJump = computed(() => shapeValid.value && !!hrefFor())

function open() {
  if (!canJump.value) return
  const href = hrefFor()
  if (!href) {
    expanded.value = true
    return
  }
  router.push(href)
}
</script>

<template>
  <div
    class="rounded-md border text-sm"
    :class="
      shapeValid
        ? 'border-[var(--cf-border)] bg-[var(--cf-surface-2,var(--cf-surface))]'
        : 'border-[var(--cf-border)] bg-[var(--cf-surface-2,var(--cf-surface))] opacity-70'
    "
    :data-testid="shapeValid ? 'citation-card-valid' : 'citation-card-invalid'"
  >
    <div class="flex flex-wrap items-center justify-between gap-2 px-2.5 py-1.5">
      <button
        type="button"
        class="min-w-0 flex-1 truncate text-left"
        :class="shapeValid ? 'hover:underline' : 'cursor-default text-txt3'"
        @click="expanded = !expanded"
      >
        <span
          class="mr-1.5 rounded px-1.5 py-0.5 text-[10px] uppercase tracking-wide"
          :class="shapeValid ? 'bg-[var(--cf-border)]' : 'bg-elevated text-txt3'"
        >
          {{ citation.type }}
        </span>
        <span class="font-medium text-txt">{{ mainLabel }}</span>
        <span v-if="subLabel" class="mt-0.5 block truncate text-[11px] font-normal text-txt3">{{ subLabel }}</span>
      </button>
      <button
        type="button"
        class="shrink-0 text-xs"
        :class="canJump ? 'text-[var(--cf-accent,#2563eb)] hover:underline' : 'cursor-not-allowed text-txt3'"
        :disabled="!canJump"
        :aria-disabled="!canJump"
        data-testid="citation-open-source"
        @click="open"
      >
        {{ t('pages.projectDetail.pm.openSource') }}
      </button>
      <p
        v-if="!shapeValid"
        class="basis-full text-[11px] text-rose-600"
        data-testid="citation-invalid-note"
      >
        {{ t('pages.projectDetail.pm.citationInvalid') }}
      </p>
    </div>
    <div v-if="expanded" class="border-t border-[var(--cf-border)] px-2.5 py-2 text-xs text-[var(--cf-muted)]">
      <pre v-if="citation.detail" class="whitespace-pre-wrap break-all">{{ citation.detail }}</pre>
      <p v-else>{{ citation.summarySnippet || citation.targetId }}</p>
      <p v-if="shapeValid && !hrefFor()" class="mt-1 text-rose-600">{{ t('pages.projectDetail.pm.citationMissing') }}</p>
    </div>
  </div>
</template>

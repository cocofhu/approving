<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import type { ProgressCitation } from '@/lib/types'

const props = defineProps<{ citation: ProgressCitation }>()
const { t } = useI18n()
const router = useRouter()
const expanded = ref(false)

const label = computed(() => {
  const c = props.citation
  return c.summarySnippet || `${c.type}: ${c.targetId}`
})

function hrefFor(): string | null {
  const c = props.citation
  switch (c.type) {
    case 'run':
      return `/runs/${c.targetId}`
    case 'gate':
      // gate targetId may be runId:nodeId or just runId
      if (c.targetId.includes(':')) {
        const [runId] = c.targetId.split(':')
        return `/runs/${runId}`
      }
      return `/runs/${c.targetId}`
    case 'artifact':
      return `/artifacts?q=${encodeURIComponent(c.targetId)}`
    case 'workflow':
      return `/workflows/${c.targetId}`
    case 'plan':
      return `/runs/${c.targetId}`
    default:
      return null
  }
}

function open() {
  const href = hrefFor()
  if (!href) {
    expanded.value = true
    return
  }
  router.push(href)
}
</script>

<template>
  <div class="rounded-md border border-[var(--cf-border)] bg-[var(--cf-surface-2,var(--cf-surface))] text-sm">
    <div class="flex items-center justify-between gap-2 px-2.5 py-1.5">
      <button type="button" class="min-w-0 flex-1 truncate text-left hover:underline" @click="expanded = !expanded">
        <span class="mr-1.5 rounded bg-[var(--cf-border)] px-1.5 py-0.5 text-[10px] uppercase tracking-wide">
          {{ citation.type }}
        </span>
        {{ label }}
      </button>
      <button
        type="button"
        class="shrink-0 text-xs text-[var(--cf-accent,#2563eb)] hover:underline"
        @click="open"
      >
        {{ t('pages.projectDetail.pm.openSource') }}
      </button>
    </div>
    <div v-if="expanded" class="border-t border-[var(--cf-border)] px-2.5 py-2 text-xs text-[var(--cf-muted)]">
      <pre v-if="citation.detail" class="whitespace-pre-wrap break-all">{{ citation.detail }}</pre>
      <p v-else>{{ citation.summarySnippet || citation.targetId }}</p>
      <p v-if="!hrefFor()" class="mt-1 text-rose-600">{{ t('pages.projectDetail.pm.citationMissing') }}</p>
    </div>
  </div>
</template>

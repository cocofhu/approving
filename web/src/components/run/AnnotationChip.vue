<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import type { ReactAnnotation } from '@/lib/types'
import { isQuoteAnnotation } from '@/lib/reviewQuote'

const props = withDefaults(
  defineProps<{
    ann: ReactAnnotation
    removable?: boolean
    testId?: string
  }>(),
  { removable: false },
)

defineEmits<{ remove: [] }>()

const { t } = useI18n()

const kind = computed<'field' | 'quote' | 'unbound'>(() => {
  if (!isQuoteAnnotation(props.ann)) return 'field'
  return props.ann.jsonPath || props.ann.selector ? 'quote' : 'unbound'
})

const chipClass = computed(() => {
  if (kind.value === 'field') return 'border-accent/40 bg-accent/10 text-accent-2'
  if (kind.value === 'quote') return 'border-ok/40 bg-ok/10 text-ok'
  return 'border-txt2/35 bg-txt2/8 text-txt2'
})

const kindLabel = computed(() => {
  if (kind.value === 'field') return t('pages.reviewComposer.chipField')
  if (kind.value === 'quote') return t('pages.reviewComposer.chipQuote')
  return t('pages.reviewComposer.chipUnbound')
})

const path = computed(() => props.ann.jsonPath || props.ann.selector || '')
</script>

<template>
  <span
    class="inline-flex max-w-full items-start gap-1 border px-1.5 py-0.5 text-[11px] leading-snug"
    :class="chipClass"
    :title="ann.note || path || ann.quote || ''"
    :data-testid="testId"
    :data-chip-kind="kind"
  >
    <Icon v-if="kind === 'field'" name="crosshair" :size="10" class="mt-0.5 shrink-0" />
    <span v-else class="mt-0.5 shrink-0 text-[12px] leading-none" aria-hidden="true">❝</span>
    <span class="flex min-w-0 flex-col gap-0.5">
      <span class="text-[9px] font-semibold uppercase tracking-wide opacity-75">{{ kindLabel }}</span>
      <span v-if="path" class="truncate font-mono text-[9px] text-txt3" :title="path">{{ path }}</span>
      <span v-if="kind !== 'field'" class="break-words [display:-webkit-box] [-webkit-box-orient:vertical] [-webkit-line-clamp:3] overflow-hidden">
        {{ ann.quote }}
        <span
          v-if="ann.truncated"
          class="ml-1 inline-block border border-current px-0.5 text-[9px] opacity-80"
        >{{ t('pages.reviewComposer.chipTruncated') }}</span>
      </span>
      <span v-else class="truncate">{{ ann.label || path }}</span>
    </span>
    <button
      v-if="removable"
      type="button"
      class="mt-0.5 shrink-0 text-txt3 hover:text-txt"
      :aria-label="t('pages.reviewComposer.removeChip')"
      @click="$emit('remove')"
    >
      <Icon name="close" :size="9" />
    </button>
  </span>
</template>

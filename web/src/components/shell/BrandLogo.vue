<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { detectLocale } from '@/lib/shared/locale'

const props = withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
    align?: 'start' | 'center'
  }>(),
  {
    size: 'sm',
    align: 'start',
  },
)

const { t, te } = useI18n()

const rootClass = computed(() => [
  'brand-logo',
  { sm: 'brand-logo--sm', md: 'brand-logo--md', lg: 'brand-logo--lg' }[props.size],
  props.align === 'center' ? 'items-center text-center' : '',
])

/** Both locales use "Approving"; keep literal fallback so brand paints before locale JSON. */
const appName = computed(() => (te('shell.appName') ? String(t('shell.appName')) : 'Approving'))
const TAGLINE_FALLBACK = {
  'zh-CN': '开发工作流编排',
  en: 'Dev workflow orchestration',
} as const
const tagline = computed(() =>
  te('shell.tagline') ? String(t('shell.tagline')) : TAGLINE_FALLBACK[detectLocale()],
)
</script>

<template>
  <div :class="rootClass">
    <span class="brand-logo__name">{{ appName }}</span>
    <span class="brand-logo__tagline">{{ tagline }}</span>
  </div>
</template>

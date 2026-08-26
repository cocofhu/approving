<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { detectLocale } from '@/lib/shared/locale'

const props = withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
    align?: 'start' | 'center'
    /** Desktop AppSidebar only: 32px purple-indigo checkmark square. */
    showMark?: boolean
  }>(),
  {
    size: 'sm',
    align: 'start',
    showMark: false,
  },
)

const { t, te } = useI18n()

const rootClass = computed(() => [
  'brand-logo',
  { sm: 'brand-logo--sm', md: 'brand-logo--md', lg: 'brand-logo--lg' }[props.size],
  props.align === 'center' ? 'items-center text-center' : '',
  props.showMark ? 'brand-logo--with-mark' : '',
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
    <span v-if="showMark" class="brand-logo__mark" aria-hidden="true">
      <svg class="brand-logo__check" width="18" height="18" viewBox="0 0 18 18" fill="none" aria-hidden="true">
        <path d="M3.2 9.2 7.1 13 14.8 4.6" stroke="#fff" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </span>
    <div class="brand-logo__text">
      <span class="brand-logo__name">{{ appName }}</span>
      <span v-if="!showMark" class="brand-logo__tagline">{{ tagline }}</span>
    </div>
  </div>
</template>

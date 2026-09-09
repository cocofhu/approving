<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { detectLocale } from '@/lib/shared/locale'
import { useBrandSettings } from '@/lib/composables/useBrandSettings'

const props = withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
    align?: 'start' | 'center'
    /** When false, hide tagline (desktop sidebar compact wordmark). */
    showTagline?: boolean
    /** Opt in for authenticated shell locations; login keeps built-in branding. */
    useCustomBrand?: boolean
  }>(),
  {
    size: 'sm',
    align: 'start',
    showTagline: true,
    useCustomBrand: false,
  },
)

const { t, te } = useI18n()
const { productName } = useBrandSettings()

const rootClass = computed(() => [
  'brand-logo',
  { sm: 'brand-logo--sm', md: 'brand-logo--md', lg: 'brand-logo--lg' }[props.size],
  props.align === 'center' ? 'items-center text-center' : '',
])

/** Both locales use "Approving"; keep literal fallback so brand paints before locale JSON. */
const appName = computed(() => {
  if (props.useCustomBrand) return productName.value
  return te('shell.appName') ? String(t('shell.appName')) : 'Approving'
})
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
    <div class="brand-logo__text">
      <span class="brand-logo__name">{{ appName }}</span>
      <span v-if="showTagline" class="brand-logo__tagline">{{ tagline }}</span>
    </div>
  </div>
</template>

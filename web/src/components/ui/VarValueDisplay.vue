<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import {
  compositeDisplayText,
  compositeImages,
  formatVarValue,
  isCompositeText,
} from '@/lib/compositeText'

const props = withDefaults(
  defineProps<{
    value: unknown
    /** Tighter layout for timeline chips. */
    compact?: boolean
    /** Parent already shows thumbnails — hide the image-count badge. */
    hideImageBadge?: boolean
    quote?: boolean
    /** Show 是/否 instead of true/false. */
    localeBool?: boolean
    /** Preserve line breaks for multi-line paragraph text. */
    preWrap?: boolean
  }>(),
  { compact: false, hideImageBadge: false, quote: false, localeBool: false, preWrap: false },
)

const { t } = useI18n()

const text = computed(() => {
  if (props.value == null || props.value === '') return null
  if (typeof props.value === 'boolean') {
    if (props.localeBool) return props.value ? t('common.bool.yes') : t('common.bool.no')
    return props.value ? t('common.bool.true') : t('common.bool.false')
  }
  if (typeof props.value === 'string') return props.preWrap ? props.value || null : props.value.trim() || null
  if (isCompositeText(props.value)) {
    const display = compositeDisplayText(props.value)
    return props.preWrap ? display || null : display.trim() || null
  }
  return null
})

const imageCount = computed(() => compositeImages(props.value).length)
const showBadge = computed(() => !props.hideImageBadge && imageCount.value > 0)
const imageBadgeTitle = computed(() => t('common.format.imageCountBadge', { n: imageCount.value }))

const fallback = computed(() => {
  const v = props.value
  if (v == null || v === '') return t('common.format.empty')
  if (typeof v === 'boolean') {
    if (props.localeBool) return v ? t('common.bool.yes') : t('common.bool.no')
    return v ? t('common.bool.true') : t('common.bool.false')
  }
  if (Array.isArray(v)) return v.length ? v.join(', ') : t('common.format.empty')
  if (isCompositeText(v)) {
    const display = (v.text || '').trim()
    const n = v.images?.length ?? 0
    if (display) return display
    if (n) return t('common.format.imageCountFull', { n })
    return t('common.format.empty')
  }
  return formatVarValue(v)
})

const displayText = computed(() => {
  if (text.value != null) return props.quote ? `"${text.value}"` : text.value
  return null
})

const textClass = computed(() => {
  const base = props.preWrap ? 'min-w-0 whitespace-pre-wrap break-words' : 'min-w-0 truncate'
  return base
})
</script>

<template>
  <span class="inline-flex min-w-0 items-center gap-1.5" :class="compact ? 'max-w-full' : ''">
    <span v-if="displayText" :class="textClass">{{ displayText }}</span>
    <span v-else-if="!showBadge" :class="[textClass, 'text-txt3']">{{ fallback }}</span>
    <span
      v-if="showBadge"
      class="inline-flex shrink-0 items-center gap-0.5 rounded-full border border-info/25 bg-info/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-info"
      :title="imageBadgeTitle"
    >
      <Icon name="paperclip" :size="10" />
      <span>{{ imageCount }}</span>
    </span>
  </span>
</template>

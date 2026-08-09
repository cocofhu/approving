<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    src: string
    label: string
    /** previewable: click opens preview; locked: ReAct agent/draft guard */
    mode?: 'previewable' | 'locked'
    size?: 'md' | 'sm' | 'xs'
    /** Extra classes (e.g. rounded-md) so each surface keeps its corner rules. */
    thumbClass?: string
    testId?: string
    alt?: string
  }>(),
  { mode: 'previewable', size: 'md', thumbClass: '', alt: '' },
)

const emit = defineEmits<{ preview: [] }>()
const { t } = useI18n()

const sizeClass = computed(() => {
  if (props.size === 'sm') return 'h-14 w-14'
  if (props.size === 'xs') return 'h-8 w-8'
  return 'h-20 w-20'
})

const hintClass = computed(() =>
  props.size === 'xs'
    ? 'text-[8px] leading-tight'
    : 'text-[10px] leading-tight',
)

function onPreviewClick() {
  if (props.mode === 'previewable') emit('preview')
}
</script>

<template>
  <button
    v-if="mode === 'previewable'"
    type="button"
    class="group relative cursor-pointer overflow-hidden border border-line transition hover:border-accent"
    :class="[sizeClass, thumbClass]"
    :data-testid="testId"
    :aria-label="t('common.chatImage.previewAria', { label })"
    @click="onPreviewClick"
  >
    <img :src="src" class="h-full w-full object-cover" :alt="alt" />
    <span
      class="pointer-events-none absolute inset-x-0 bottom-0 bg-black/55 px-1 py-0.5 text-center text-white opacity-0 transition-opacity group-hover:opacity-100"
      :class="hintClass"
    >{{ t('common.chatImage.clickToEnlarge') }}</span>
  </button>
  <div
    v-else
    class="relative overflow-hidden border border-line"
    :class="[sizeClass, thumbClass]"
    :data-testid="testId"
  >
    <img :src="src" class="h-full w-full object-cover" :alt="alt" />
    <span
      class="pointer-events-none absolute inset-x-0 bottom-0 bg-black/45 px-0.5 py-0.5 text-center text-[9px] leading-tight text-txt2"
    >{{ t('common.chatImage.previewUnavailable') }}</span>
  </div>
</template>

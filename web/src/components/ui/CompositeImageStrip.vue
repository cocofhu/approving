<script setup lang="ts">
import { computed } from 'vue'
import { compositeImages, imgSrc } from '@/lib/shared/compositeText'

const props = withDefaults(
  defineProps<{
    value: unknown
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { size: 'sm' },
)

const SIZE_CLS: Record<string, string> = {
  sm: 'h-10 w-10',
  md: 'h-14 w-14',
  lg: 'h-12 w-12',
}

const images = computed(() => compositeImages(props.value))
const sizeClass = computed(() => SIZE_CLS[props.size] ?? SIZE_CLS.sm)
</script>

<template>
  <div v-if="images.length" class="flex flex-wrap gap-1.5">
    <img
      v-for="(im, ii) in images"
      :key="ii"
      :src="imgSrc(im)"
      :class="[sizeClass, 'rounded border border-line object-cover']"
    />
  </div>
</template>

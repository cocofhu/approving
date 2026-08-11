<script setup lang="ts">
import { computed } from 'vue'
import VarValueDisplay from '@/components/ui/VarValueDisplay.vue'
import CompositeImageStrip from '@/components/ui/CompositeImageStrip.vue'
import { compositeImages } from '@/lib/shared/compositeText'

const props = withDefaults(
  defineProps<{
    value: unknown
    localeBool?: boolean
    preWrap?: boolean
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { localeBool: false, preWrap: false, size: 'sm' },
)

const images = computed(() => compositeImages(props.value))
const stripMargin = computed(() => (props.size === 'md' ? 'mt-1.5' : 'mt-1'))
</script>

<template>
  <div>
    <VarValueDisplay
      :value="value"
      :locale-bool="localeBool"
      :pre-wrap="preWrap"
      :hide-image-badge="images.length > 0"
    />
    <CompositeImageStrip :value="value" :size="size" :class="stripMargin" />
  </div>
</template>

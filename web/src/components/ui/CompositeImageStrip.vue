<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppSpinner from './AppSpinner.vue'
import { compositeImages, imgSrc } from '@/lib/compositeText'

type ThumbStatus = 'loading' | 'ok' | 'failed'

const props = withDefaults(
  defineProps<{
    value: unknown
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { size: 'sm' },
)

const { t } = useI18n()

const SIZE_CLS: Record<string, string> = {
  sm: 'h-10 w-10',
  md: 'h-14 w-14',
  lg: 'h-12 w-12',
}

/** Fail/loading cards need room for copy (Demo: ~140×72). */
const CARD_CLS: Record<string, string> = {
  sm: 'min-h-[4.5rem] w-[7.5rem]',
  md: 'min-h-[5rem] w-36',
  lg: 'min-h-[4.75rem] w-32',
}

const images = computed(() => compositeImages(props.value))
const sizeClass = computed(() => SIZE_CLS[props.size] ?? SIZE_CLS.sm)
const cardClass = computed(() => CARD_CLS[props.size] ?? CARD_CLS.sm)

/** Per-index status after mount; empty src never enters loading. */
const statuses = reactive<Record<number, ThumbStatus>>({})

const items = computed(() =>
  images.value.map((im, index) => {
    const src = imgSrc(im)
    const status: ThumbStatus = !src ? 'failed' : (statuses[index] ?? 'loading')
    return { index, src, status }
  }),
)

watch(
  images,
  (imgs) => {
    for (const key of Object.keys(statuses)) {
      delete statuses[Number(key)]
    }
    imgs.forEach((im, index) => {
      statuses[index] = imgSrc(im) ? 'loading' : 'failed'
    })
  },
  { immediate: true, deep: true },
)

function onLoad(index: number) {
  statuses[index] = 'ok'
}

function onError(index: number) {
  statuses[index] = 'failed'
}
</script>

<template>
  <div v-if="items.length" class="flex flex-wrap gap-1.5" data-testid="composite-image-strip">
    <template v-for="item in items" :key="item.index">
      <!-- Permanent failure: empty src or blob/@error; no retry, no HTTP/path details -->
      <div
        v-if="item.status === 'failed'"
        class="flex flex-col items-center justify-center gap-1 border border-err/35 bg-err/[0.06] px-2 py-2 text-center"
        :class="cardClass"
        data-image-failed="1"
        data-testid="composite-image-failed"
        role="img"
        :aria-label="`${t('common.compositeImage.cannotDisplay')} · ${t('common.compositeImage.attachmentUnavailable')}`"
      >
        <div class="text-[11px] font-semibold text-red-300">
          {{ t('common.compositeImage.cannotDisplay') }}
        </div>
        <div class="text-[11px] leading-snug text-txt3">
          {{ t('common.compositeImage.attachmentUnavailable') }}
        </div>
      </div>

      <!-- Has src: loading overlay until @load; @error → failed -->
      <div
        v-else
        class="relative overflow-hidden border border-line"
        :class="item.status === 'loading' ? cardClass : sizeClass"
        :data-testid="item.status === 'loading' ? 'composite-image-loading' : 'composite-image-ok'"
        :role="item.status === 'loading' ? 'status' : undefined"
        :aria-label="item.status === 'loading' ? t('common.compositeImage.loading') : undefined"
      >
        <div
          v-if="item.status === 'loading'"
          class="absolute inset-0 z-[1] flex flex-col items-center justify-center gap-1 bg-elevated text-[11px] text-txt3"
          aria-hidden="true"
        >
          <AppSpinner :size="12" />
          <span>{{ t('common.compositeImage.loading') }}</span>
        </div>
        <img
          :src="item.src"
          class="h-full w-full object-cover"
          :class="item.status === 'loading' ? 'opacity-0' : ''"
          alt=""
          @load="onLoad(item.index)"
          @error="onError(item.index)"
        />
      </div>
    </template>
  </div>
</template>

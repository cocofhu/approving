<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppSpinner from './AppSpinner.vue'
import { compositeImages, imgSrc } from '@/lib/shared/compositeText'
import {
  beginAutoLoad,
  isKnownLoaded,
  isKnownMissing,
  markLoaded,
  markMissing,
  parseBlobId,
  subscribe,
} from '@/lib/shared/blobMissingCache'
import type { ClarifyImage } from '@/lib/shared/types'

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

/** Per stable blob-id / src fingerprint status; survives poll object replacement. */
const statuses = reactive<Record<string, ThumbStatus>>({})
/** When false, do not mount <img> (known missing or waiting on another surface's inflight). */
const allowImg = reactive<Record<string, boolean>>({})
const cacheTick = ref(0)

function stableKey(im: ClarifyImage, index: number): string {
  const ref = (im.ref || '').trim()
  if (ref.startsWith('blob:')) {
    const id = parseBlobId(ref)
    if (id) return `blob:${id}`
  }
  const src = imgSrc(im)
  if (src) {
    const id = parseBlobId(src)
    if (id) return `blob:${id}`
    return `src:${src}`
  }
  return `idx:${index}`
}

function resolveBlobId(im: ClarifyImage, src: string): string | null {
  return parseBlobId((im.ref || '').trim()) || parseBlobId(src)
}

function applyImage(im: ClarifyImage, index: number) {
  const key = stableKey(im, index)
  const src = imgSrc(im)
  if (!src) {
    statuses[key] = 'failed'
    allowImg[key] = false
    return
  }
  const blobId = resolveBlobId(im, src)

  if (blobId && isKnownMissing(blobId)) {
    statuses[key] = 'failed'
    allowImg[key] = false
    return
  }

  // Stable reconcile: keep ok/failed across poll unless chat retry succeeded (knownLoaded).
  if (statuses[key] === 'ok') {
    allowImg[key] = true
    return
  }
  if (statuses[key] === 'failed') {
    if (blobId && isKnownLoaded(blobId)) {
      statuses[key] = 'ok'
      allowImg[key] = true
      return
    }
    allowImg[key] = false
    return
  }

  // New key or still loading
  if (blobId) {
    const decision = beginAutoLoad(blobId)
    if (decision === 'blocked_missing') {
      statuses[key] = 'failed'
      allowImg[key] = false
      return
    }
    if (decision === 'blocked_pending') {
      statuses[key] = 'loading'
      allowImg[key] = false
      return
    }
  }
  statuses[key] = 'loading'
  allowImg[key] = true
}

function reconcile() {
  void cacheTick.value
  const imgs = images.value
  const nextKeys = new Set<string>()
  imgs.forEach((im, index) => {
    const key = stableKey(im, index)
    nextKeys.add(key)
    applyImage(im, index)
  })
  for (const key of Object.keys(statuses)) {
    if (!nextKeys.has(key)) {
      delete statuses[key]
      delete allowImg[key]
    }
  }
}

watch(images, reconcile, { immediate: true, deep: true })

const unsub = subscribe(() => {
  cacheTick.value += 1
  reconcile()
})
onUnmounted(unsub)

const items = computed(() => {
  void cacheTick.value
  return images.value.map((im, index) => {
    const key = stableKey(im, index)
    const src = imgSrc(im)
    const status: ThumbStatus = !src ? 'failed' : (statuses[key] ?? 'loading')
    const showImg = !!src && status !== 'failed' && allowImg[key] !== false
    return { key, src, status, showImg }
  })
})

function onLoad(key: string, src: string) {
  statuses[key] = 'ok'
  allowImg[key] = true
  const id = parseBlobId(src)
  if (id) markLoaded(id)
}

function onError(key: string, src: string) {
  statuses[key] = 'failed'
  allowImg[key] = false
  const id = parseBlobId(src)
  if (id) markMissing(id)
}
</script>

<template>
  <div v-if="items.length" class="flex flex-wrap gap-1.5" data-testid="composite-image-strip">
    <template v-for="item in items" :key="item.key">
      <!-- Permanent failure: empty src, known-missing, or @error; no retry, no HTTP/path details -->
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

      <!-- Has src: loading overlay until @load; @error → failed. showImg gates orphan auto GET. -->
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
          v-if="item.showImg"
          :src="item.src"
          class="h-full w-full object-cover"
          :class="item.status === 'loading' ? 'opacity-0' : ''"
          alt=""
          @load="onLoad(item.key, item.src)"
          @error="onError(item.key, item.src)"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, watch } from 'vue'
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

/** Loading must reach a terminal state; 12s is within the agreed 8–15s band. */
const LOAD_TIMEOUT_MS = 12_000

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

type SlotState = {
  src: string
  status: ThumbStatus
  showImg: boolean
}

/** Per stable blob-id / src fingerprint slot; survives poll object replacement. */
const slots = reactive<Record<string, SlotState>>({})
const loadTimers = new Map<string, ReturnType<typeof setTimeout>>()
const imgEls = new Map<string, HTMLImageElement>()

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

function clearTimer(key: string) {
  const timer = loadTimers.get(key)
  if (timer != null) {
    clearTimeout(timer)
    loadTimers.delete(key)
  }
}

function clearAllTimers() {
  for (const timer of loadTimers.values()) clearTimeout(timer)
  loadTimers.clear()
}

function upsertSlot(key: string, src: string, status: ThumbStatus, showImg: boolean) {
  const slot = slots[key]
  if (slot) {
    slot.src = src
    slot.status = status
    slot.showImg = showImg
  } else {
    slots[key] = { src, status, showImg }
  }

  if (status === 'loading' && showImg && src) {
    clearTimer(key)
    loadTimers.set(
      key,
      setTimeout(() => {
        loadTimers.delete(key)
        const current = slots[key]
        if (current?.status === 'loading' && current.showImg && current.src) {
          onError(key, current.src)
        }
      }, LOAD_TIMEOUT_MS),
    )
    return
  }

  clearTimer(key)
}

/** Cache hit / sync decode: leave loading without waiting for a late @load. */
function syncFromImg(key: string, img: HTMLImageElement) {
  const slot = slots[key]
  if (!slot || slot.status !== 'loading' || !slot.showImg || !slot.src) return
  const shown = img.getAttribute('src') || ''
  if (shown && shown !== slot.src) return
  if (!img.complete) return
  if (img.naturalWidth > 0) {
    onLoad(key, slot.src)
    return
  }
  onError(key, slot.src)
}

function bindImg(el: unknown, key: string) {
  if (el instanceof HTMLImageElement) {
    imgEls.set(key, el)
    syncFromImg(key, el)
    return
  }
  imgEls.delete(key)
}

function reconcile() {
  const alive = new Set<string>()

  images.value.forEach((im, index) => {
    const key = stableKey(im, index)
    alive.add(key)

    const src = imgSrc(im)
    if (!src) {
      upsertSlot(key, '', 'failed', false)
      return
    }

    const blobId = resolveBlobId(im, src)
    if (blobId && isKnownLoaded(blobId)) {
      upsertSlot(key, src, 'ok', true)
      return
    }
    if (blobId && isKnownMissing(blobId)) {
      upsertSlot(key, src, 'failed', false)
      return
    }

    const prev = slots[key]
    if (prev && prev.src === src) {
      if (prev.status === 'ok') {
        upsertSlot(key, src, 'ok', true)
        return
      }
      if (prev.status === 'failed') {
        upsertSlot(key, src, 'failed', false)
        return
      }
      upsertSlot(key, src, 'loading', blobId ? prev.showImg : true)
      return
    }

    if (blobId) {
      const decision = beginAutoLoad(blobId)
      if (decision === 'blocked_missing') {
        upsertSlot(key, src, 'failed', false)
        return
      }
      if (decision === 'blocked_pending') {
        upsertSlot(key, src, 'loading', false)
        return
      }
    }

    upsertSlot(key, src, 'loading', true)
  })

  for (const key of Object.keys(slots)) {
    if (!alive.has(key)) {
      clearTimer(key)
      imgEls.delete(key)
      delete slots[key]
    }
  }
  void nextTick(() => {
    for (const [key, img] of imgEls) {
      syncFromImg(key, img)
    }
  })
}

watch(images, reconcile, { immediate: true, deep: true })

const unsub = subscribe(reconcile)

onBeforeUnmount(() => {
  clearAllTimers()
  imgEls.clear()
  unsub()
})

const items = computed(() =>
  images.value.map((im, index) => {
    const key = stableKey(im, index)
    const src = imgSrc(im)
    const slot = slots[key]
    const status: ThumbStatus = !src ? 'failed' : (slot?.status ?? 'loading')
    const showImg = !!src && status !== 'failed' && (slot?.showImg ?? true)
    return { key, src, status, showImg }
  }),
)

function onLoad(key: string, src: string) {
  upsertSlot(key, src, 'ok', true)
  const id = parseBlobId(src)
  if (id) markLoaded(id)
}

function onError(key: string, src: string) {
  upsertSlot(key, src, 'failed', false)
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

      <!-- Has src: loading overlay until load/complete/timeout; showImg gates orphan auto GET. -->
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
          :key="item.src"
          :ref="(el) => bindImg(el, item.key)"
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

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppSpinner from './AppSpinner.vue'
import { compositeImages, imgSrc } from '@/lib/shared/compositeText'

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

type SlotState = { src: string; status: ThumbStatus }

/** Per-index slot keyed by last known src so same-src poll refreshes keep ok/failed. */
const slots = reactive<Record<number, SlotState>>({})
const loadTimers = new Map<number, ReturnType<typeof setTimeout>>()
const imgEls = new Map<number, HTMLImageElement>()

const items = computed(() =>
  images.value.map((im, index) => {
    const src = imgSrc(im)
    const slot = slots[index]
    const status: ThumbStatus = !src
      ? 'failed'
      : (slot?.status ?? 'loading')
    return { index, src, status }
  }),
)

function clearTimer(index: number) {
  const timer = loadTimers.get(index)
  if (timer != null) {
    clearTimeout(timer)
    loadTimers.delete(index)
  }
}

function clearAllTimers() {
  for (const timer of loadTimers.values()) clearTimeout(timer)
  loadTimers.clear()
}

function setStatus(index: number, status: ThumbStatus) {
  const slot = slots[index]
  if (!slot) return
  if (status === 'ok' || status === 'failed') clearTimer(index)
  slot.status = status
}

function startLoadTimeout(index: number) {
  clearTimer(index)
  loadTimers.set(
    index,
    setTimeout(() => {
      loadTimers.delete(index)
      if (slots[index]?.status === 'loading') {
        slots[index].status = 'failed'
      }
    }, LOAD_TIMEOUT_MS),
  )
}

/** Cache hit / sync decode: leave loading without waiting for a late @load. */
function syncFromImg(index: number, img: HTMLImageElement) {
  const slot = slots[index]
  if (!slot || slot.status !== 'loading' || !slot.src) return
  // Ignore stale complete from a previous src on the same element (before remount).
  const shown = img.getAttribute('src') || ''
  if (shown && shown !== slot.src) return
  if (!img.complete) return
  if (img.naturalWidth > 0) {
    setStatus(index, 'ok')
    return
  }
  // complete with no dimensions → broken resource
  setStatus(index, 'failed')
}

function bindImg(el: unknown, index: number) {
  if (el instanceof HTMLImageElement) {
    imgEls.set(index, el)
    syncFromImg(index, el)
    return
  }
  imgEls.delete(index)
}

watch(
  images,
  (imgs) => {
    const alive = new Set<number>()
    imgs.forEach((im, index) => {
      alive.add(index)
      const src = imgSrc(im)
      const prev = slots[index]

      if (!src) {
        clearTimer(index)
        slots[index] = { src: '', status: 'failed' }
        return
      }

      // Same src: keep terminal ok/failed (Run detail 2s poll replaces value objects).
      if (prev && prev.src === src) {
        if (prev.status === 'loading' && !loadTimers.has(index)) {
          startLoadTimeout(index)
        }
        return
      }

      // Empty→src or src change: new loading transition.
      slots[index] = { src, status: 'loading' }
      startLoadTimeout(index)
    })

    for (const key of Object.keys(slots)) {
      const index = Number(key)
      if (!alive.has(index)) {
        clearTimer(index)
        imgEls.delete(index)
        delete slots[index]
      }
    }

    void nextTick(() => {
      for (const [index, img] of imgEls) {
        syncFromImg(index, img)
      }
    })
  },
  { immediate: true, deep: true },
)

onBeforeUnmount(() => {
  clearAllTimers()
  imgEls.clear()
})

function onLoad(index: number) {
  setStatus(index, 'ok')
}

function onError(index: number) {
  setStatus(index, 'failed')
}
</script>

<template>
  <div v-if="items.length" class="flex flex-wrap gap-1.5" data-testid="composite-image-strip">
    <template v-for="item in items" :key="item.index">
      <!-- Permanent failure: empty src or blob/@error/timeout; no retry, no HTTP/path details -->
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

      <!-- Has src: loading overlay until load/complete/timeout; @error → failed -->
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
          :key="item.src"
          :ref="(el) => bindImg(el, item.index)"
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

<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  open: boolean
  selector: string
  imageDataUrl?: string
  screenshotMissing?: boolean
  /** Initial comment when editing an existing pin. */
  initialComment?: string
  /**
   * Anchor rect relative to the positioning container (preview wrap).
   * Used by placeCardNear to flip/clamp inside the container.
   */
  anchor?: { left: number; top: number; width: number; height: number } | null
  containerEl?: HTMLElement | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', comment: string): void
}>()

const { t } = useI18n()
const cardRef = ref<HTMLElement | null>(null)
const comment = ref('')
const top = ref(8)
const left = ref(8)

watch(
  () => props.open,
  (v) => {
    if (v) {
      comment.value = props.initialComment || ''
      nextTick(() => placeCardNear())
    }
  },
)

watch(
  () => [props.anchor, props.initialComment] as const,
  () => {
    if (!props.open) return
    if (props.initialComment != null) comment.value = props.initialComment
    nextTick(() => placeCardNear())
  },
)

function placeCardNear() {
  const card = cardRef.value
  const wrap = props.containerEl
  if (!card || !wrap) return
  const wrapRect = wrap.getBoundingClientRect()
  const cardH = card.offsetHeight || 280
  const cardW = card.offsetWidth || 280
  const a = props.anchor
  let below = 8
  let aboveCandidate = 8
  let preferLeft = 8
  if (a) {
    below = a.top + a.height + 8
    aboveCandidate = a.top - cardH - 8
    preferLeft = a.left
  }
  let nextTop = below
  if (below + cardH > wrapRect.height - 8) {
    nextTop = aboveCandidate >= 8 ? aboveCandidate : Math.max(8, wrapRect.height - cardH - 8)
  }
  let nextLeft = preferLeft
  if (nextLeft + cardW > wrapRect.width - 8) nextLeft = wrapRect.width - cardW - 8
  if (nextLeft < 8) nextLeft = 8
  top.value = nextTop
  left.value = nextLeft
}

function onSave() {
  const body = comment.value.trim()
  if (!body) return
  emit('save', body)
}

function onKeydown(ev: KeyboardEvent) {
  if (!props.open) return
  if (ev.key === 'Escape') {
    ev.preventDefault()
    emit('close')
  }
}

onMounted(() => {
  window.addEventListener('resize', placeCardNear)
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', placeCardNear)
  window.removeEventListener('keydown', onKeydown)
})

defineExpose({ placeCardNear })
</script>

<template>
  <div
    v-show="open"
    ref="cardRef"
    class="absolute z-20 flex max-h-[calc(100%-24px)] w-[280px] flex-col border border-line-strong bg-elevated shadow-[var(--shadow-card)]"
    :style="{ top: top + 'px', left: left + 'px' }"
    data-testid="comment-pin-inspect-card"
    role="dialog"
    :aria-label="t('pages.gateApproval.commentPins.inspectTitle')"
  >
    <div class="flex shrink-0 items-center justify-between gap-2 border-b border-line px-3 py-2.5">
      <div class="min-w-0">
        <div class="text-xs font-semibold text-txt">
          {{ t('pages.gateApproval.commentPins.inspectTitle') }}
        </div>
        <div class="truncate font-mono text-[11px] text-txt3" :title="selector">
          {{ selector || '—' }}
        </div>
      </div>
      <button
        type="button"
        class="border border-line px-2 py-1 text-[11px] text-txt2 hover:text-txt"
        data-testid="comment-pin-card-close"
        @click="emit('close')"
      >
        {{ t('common.buttons.close') }}
      </button>
    </div>
    <div class="flex min-h-0 flex-1 flex-col gap-2 overflow-auto px-3 py-2.5">
      <div
        class="flex h-11 items-center justify-center border border-dashed border-line-strong bg-base text-[11px]"
        :class="
          screenshotMissing
            ? 'border-warn/50 text-warn'
            : imageDataUrl
              ? 'border-solid text-accent'
              : 'text-txt3'
        "
        data-testid="comment-pin-thumb"
      >
        <img
          v-if="imageDataUrl && !screenshotMissing"
          :src="imageDataUrl"
          alt=""
          class="h-full max-w-full object-contain"
        />
        <span v-else-if="screenshotMissing">
          {{ t('pages.gateApproval.commentPins.noScreenshot') }}
        </span>
        <span v-else>{{ t('pages.gateApproval.commentPins.shotPreview') }}</span>
      </div>
      <textarea
        v-model="comment"
        class="min-h-[56px] w-full resize-y border border-line bg-base px-2.5 py-2 text-[13px] text-txt outline-none focus:border-accent"
        rows="3"
        :placeholder="t('pages.gateApproval.commentPins.placeholder')"
        data-testid="comment-pin-input"
      />
      <p class="text-[11px] text-txt3">
        {{ t('pages.gateApproval.commentPins.hint') }}
      </p>
    </div>
    <div class="shrink-0 border-t border-line bg-elevated px-3 py-2.5">
      <button
        type="button"
        class="w-full bg-accent px-3 py-2 text-xs font-medium text-white hover:bg-accent-2 disabled:cursor-not-allowed disabled:opacity-45"
        :disabled="!comment.trim()"
        data-testid="comment-pin-save"
        @click="onSave"
      >
        {{ t('pages.gateApproval.commentPins.savePin') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppModal from './AppModal.vue'
import {
  isKnownLoaded,
  isKnownMissing,
  markLoaded,
  markMissing,
  parseBlobId,
  subscribe,
} from '@/lib/shared/blobMissingCache'

const props = withDefaults(
  defineProps<{
    open: boolean
    src: string
    label: string
    /** Prefix for data-testid (e.g. clarify-image-preview). */
    testIdPrefix?: string
  }>(),
  { testIdPrefix: 'chat-image-preview' },
)

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const loadFailed = ref(false)
const cacheTick = ref(0)

const blobId = computed(() => parseBlobId(props.src))

/** Known-missing: never mount a requesting <img> (Demo preview gate). */
const blockedMissing = computed(() => {
  void cacheTick.value
  const id = blobId.value
  return !!id && isKnownMissing(id)
})

const showFailed = computed(() => loadFailed.value || blockedMissing.value)

watch(
  () => [props.open, props.src] as const,
  () => {
    loadFailed.value = false
  },
)

const unsub = subscribe(() => {
  cacheTick.value += 1
  const id = blobId.value
  if (id && isKnownMissing(id)) {
    loadFailed.value = true
  } else if (id && isKnownLoaded(id)) {
    // Chat retry success while preview open — allow img again.
    loadFailed.value = false
  }
})
onUnmounted(unsub)

const title = computed(() =>
  props.label ? t('common.chatImage.previewTitle', { label: props.label }) : '',
)

const ids = computed(() => ({
  body: `${props.testIdPrefix}-body`,
  img: `${props.testIdPrefix}-img`,
  failed: `${props.testIdPrefix}-failed`,
}))

function onError() {
  loadFailed.value = true
  const id = blobId.value
  if (id) markMissing(id)
}

function onLoad() {
  loadFailed.value = false
  const id = blobId.value
  if (id) markLoaded(id)
}
</script>

<template>
  <AppModal :open="open" :title="title" :width="960" @close="emit('close')">
    <div
      v-if="open"
      class="flex min-h-[280px] items-center justify-center"
      :data-testid="ids.body"
    >
      <div
        v-if="showFailed"
        class="px-4 py-8 text-center text-[13px] text-txt2"
        :data-testid="ids.failed"
      >
        <em class="mb-2 block font-semibold not-italic text-err">{{ t('common.chatImage.imageLoadFailed') }}</em>
        <span v-if="blockedMissing" class="block text-[12px] text-txt3">
          {{ t('common.compositeImage.attachmentUnavailable') }}
        </span>
      </div>
      <img
        v-else
        :src="src"
        :alt="label"
        class="max-h-[74vh] max-w-full object-contain"
        :data-testid="ids.img"
        @error="onError"
        @load="onLoad"
      />
    </div>
  </AppModal>
</template>

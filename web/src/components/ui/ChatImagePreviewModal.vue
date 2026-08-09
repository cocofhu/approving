<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppModal from './AppModal.vue'

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

watch(
  () => [props.open, props.src] as const,
  () => {
    loadFailed.value = false
  },
)

const title = computed(() =>
  props.label ? t('common.chatImage.previewTitle', { label: props.label }) : '',
)

const ids = computed(() => ({
  body: `${props.testIdPrefix}-body`,
  img: `${props.testIdPrefix}-img`,
  failed: `${props.testIdPrefix}-failed`,
}))
</script>

<template>
  <AppModal :open="open" :title="title" :width="960" @close="emit('close')">
    <div
      v-if="open"
      class="flex min-h-[280px] items-center justify-center"
      :data-testid="ids.body"
    >
      <div
        v-if="loadFailed"
        class="px-4 py-8 text-center text-[13px] text-txt2"
        :data-testid="ids.failed"
      >
        <em class="mb-2 block font-semibold not-italic text-err">{{ t('common.chatImage.imageLoadFailed') }}</em>
      </div>
      <img
        v-else
        :src="src"
        :alt="label"
        class="max-h-[74vh] max-w-full object-contain"
        :data-testid="ids.img"
        @error="loadFailed = true"
      />
    </div>
  </AppModal>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(
  defineProps<{
    title?: string
    message?: string
    retryLabel?: string
  }>(),
  {
    title: '',
    message: '',
    retryLabel: '',
  },
)

defineEmits<{ retry: [] }>()

const { t } = useI18n()
</script>

<template>
  <div class="border border-err/45 bg-base p-5 text-left" data-testid="app-inline-error">
    <h3 class="m-0 mb-1.5 text-sm font-semibold text-err">
      {{ title || t('common.loading.failed') }}
    </h3>
    <p v-if="message" class="m-0 mb-3 text-[13px] text-txt2">{{ message }}</p>
    <button
      type="button"
      class="inline-flex min-h-[44px] items-center justify-center bg-accent px-3 py-2 text-[13px] text-white outline-none hover:bg-accent-2 focus-visible:ring-2 focus-visible:ring-accent/40"
      data-testid="app-inline-error-retry"
      @click="$emit('retry')"
    >
      {{ retryLabel || t('common.loading.retry') }}
    </button>
  </div>
</template>

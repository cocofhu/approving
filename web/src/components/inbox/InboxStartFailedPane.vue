<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'

const emit = defineEmits<{ (e: 'dismiss'): void }>()
const { t } = useI18n()
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 px-6 text-center"
    data-testid="inbox-start-failed"
  >
    <Icon name="alert" :size="26" class="text-warn" />
    <p class="text-[13px] font-medium text-txt">{{ t('pages.gatesInbox.startFailedTitle') }}</p>
    <p class="max-w-md text-[12px] text-txt3">{{ t('pages.gatesInbox.startFailedDesc') }}</p>
    <button
      class="rounded-md border border-line bg-surface px-3 py-1.5 text-xs font-medium text-txt transition hover:bg-elevated"
      data-testid="inbox-start-failed-dismiss"
      @click="emit('dismiss')"
    >
      {{ t('pages.gatesInbox.startFailedDismiss') }}
    </button>
  </div>
</template>

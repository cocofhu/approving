<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <span
    class="inline-flex shrink-0 items-center border border-line bg-elevated px-1.5 py-0.5 text-[10px] leading-none text-txt3"
    data-testid="unknown-model-badge"
  >
    {{ t('pages.tokenByModel.unknownBadge') }}
  </span>
</template>

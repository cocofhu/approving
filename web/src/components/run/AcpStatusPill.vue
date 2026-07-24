<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

// A compact ACP session indicator: breathing dot + label. `busy` means the
// agent is actively processing a turn (运行中); `connected` but not busy means
// the session is up and waiting (空闲中); neither means it isn't attached.
const props = defineProps<{ busy?: boolean; connected?: boolean }>()

const { t } = useI18n()

const view = computed(() => {
  if (props.busy) return { label: t('pages.acpStatus.busy'), dot: 'bg-info', wrap: 'border-info/30 text-info', pulse: true }
  if (props.connected) return { label: t('pages.acpStatus.idle'), dot: 'bg-ok', wrap: 'border-ok/30 text-ok', pulse: true }
  return { label: t('pages.acpStatus.disconnected'), dot: 'bg-line-strong', wrap: 'border-line text-txt3', pulse: false }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium"
    :class="view.wrap"
  >
    <span class="relative flex h-2 w-2">
      <span
        v-if="view.pulse"
        class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-60"
        :class="view.dot"
      />
      <span class="relative inline-flex h-2 w-2 rounded-full" :class="view.dot" />
    </span>
    {{ view.label }}
  </span>
</template>

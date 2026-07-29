<script setup lang="ts">
/**
 * Shared thought <summary> status: generating / done / interrupted.
 * Bound to whole-turn busy — never treat thoughtStreaming===false as done
 * while message is still outputting.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'

export type ThoughtSummaryState = 'generating' | 'done' | 'interrupted' | 'idle'

const props = withDefaults(
  defineProps<{
    /** Whole-turn busy (thought or message still streaming / turn not finished). */
    busy?: boolean
    /** Successful completion (not interrupted). */
    completed?: boolean
    /** Interrupted or failed end. */
    interrupted?: boolean
  }>(),
  {
    busy: false,
    completed: false,
    interrupted: false,
  },
)

const { t } = useI18n()

const state = computed<ThoughtSummaryState>(() => {
  // Interrupted/failure must never look like success (no check / 已完成).
  if (props.interrupted) return 'interrupted'
  // Whole-turn busy (incl. message outputting) stays generating.
  if (props.busy) return 'generating'
  if (props.completed) return 'done'
  return 'idle'
})

const dataState = computed(() => {
  if (state.value === 'generating') return 'streaming'
  if (state.value === 'done') return 'done'
  if (state.value === 'interrupted') return 'interrupted'
  return 'idle'
})

const label = computed(() => {
  if (state.value === 'generating') return t('pages.clarify.thoughtGenerating')
  if (state.value === 'done') return t('pages.clarify.turnCompleted')
  if (state.value === 'interrupted') return t('pages.clarify.thoughtInterrupted')
  return ''
})
</script>

<template>
  <span
    class="thought-summary-status inline-flex min-w-0 flex-1 items-center gap-1.5"
    :data-state="dataState"
    data-testid="thought-summary-state"
  >
    <Icon
      name="sparkles"
      :size="11"
      class="thought-summary-sparkles shrink-0 text-accent-2"
      :class="{ 'thought-summary-sparkles--pulse': state === 'generating' }"
      aria-hidden="true"
    />
    <span class="thought-summary-title shrink-0 text-txt2">{{ t('pages.clarify.thought') }}</span>
    <span
      v-if="state !== 'idle'"
      class="thought-summary-badge ml-auto inline-flex items-center gap-1 text-[11px]"
      :class="{
        'text-accent-2': state === 'generating',
        'text-ok': state === 'done',
        'text-warn': state === 'interrupted',
      }"
    >
      <span
        v-if="state === 'generating'"
        class="thought-summary-dot"
        aria-hidden="true"
      />
      <Icon
        v-else-if="state === 'done'"
        name="check"
        :size="12"
        class="shrink-0 text-ok"
        aria-hidden="true"
      />
      <span>{{ label }}</span>
    </span>
  </span>
</template>

<style scoped>
.thought-summary-sparkles--pulse {
  animation: thought-spark-pulse 1.2s ease-in-out infinite;
}
.thought-summary-dot {
  width: 6px;
  height: 6px;
  flex-shrink: 0;
  background: currentColor;
  animation: thought-dot-pulse 1s ease-in-out infinite;
}
@keyframes thought-spark-pulse {
  0%,
  100% {
    opacity: 0.45;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.12);
  }
}
@keyframes thought-dot-pulse {
  0%,
  100% {
    opacity: 0.3;
  }
  50% {
    opacity: 1;
  }
}
@media (prefers-reduced-motion: reduce) {
  .thought-summary-sparkles--pulse,
  .thought-summary-dot {
    animation: none !important;
  }
  .thought-summary-sparkles--pulse {
    opacity: 1;
    transform: none;
  }
  .thought-summary-dot {
    opacity: 0.85;
  }
}
</style>

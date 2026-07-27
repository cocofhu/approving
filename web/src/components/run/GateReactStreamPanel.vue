<script setup lang="ts">
/**
 * Shared four-phase busy stream for GateApproval / ReviewComposer(gate):
 * placeholder → collapsible thought → outputting (shimmer+caret) → restrained done.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import { relTime } from '@/lib/format'

const props = withDefaults(
  defineProps<{
    thinking?: boolean
    streamText?: string
    streamThought?: string
    interrupted?: boolean
    /** ISO timestamp when turn completed normally (not interrupted/error). */
    completedAt?: string | null
  }>(),
  {
    thinking: false,
    streamText: '',
    streamThought: '',
    interrupted: false,
    completedAt: null,
  },
)

const { t, locale } = useI18n()

const thoughtOpenOverride = ref<boolean | undefined>(undefined)

watch(
  () => [props.streamText, props.streamThought, props.thinking, props.completedAt] as const,
  () => {
    // Reset manual override when rails / phase change so Demo defaults apply.
    thoughtOpenOverride.value = undefined
  },
)

const hasThought = computed(() => !!props.streamThought)
const hasMessage = computed(() => !!props.streamText)
const streaming = computed(() => !!props.thinking && !props.completedAt)
const showCompleted = computed(
  () =>
    !!props.completedAt &&
    !props.interrupted &&
    !props.thinking &&
    (hasThought.value || hasMessage.value),
)
const showPanel = computed(
  () =>
    props.thinking ||
    showCompleted.value ||
    (props.interrupted && (hasThought.value || hasMessage.value)),
)

/** Default open while thought-only streaming; collapse once message / done. */
const thoughtOpen = computed(() => {
  if (thoughtOpenOverride.value !== undefined) return thoughtOpenOverride.value
  return !!(streaming.value && hasThought.value && !hasMessage.value)
})

function onThoughtToggle(e: Event) {
  const el = e.target as HTMLDetailsElement
  thoughtOpenOverride.value = el.open
}
</script>

<template>
  <div
    v-if="showPanel"
    class="mt-2 max-h-40 space-y-1.5 overflow-y-auto rounded border border-line bg-surface px-2 py-1.5 text-[12px] text-txt2"
    data-testid="gate-react-stream"
  >
    <div
      v-if="streaming && !hasMessage && !hasThought"
      class="inline-flex items-center gap-2 text-txt3"
      data-testid="gate-busy-placeholder"
    >
      <span class="typing-dots" aria-hidden="true"><i /><i /><i /></span>
      <span>{{ t('pages.clarify.thinkingBusy') }}</span>
    </div>
    <div
      v-else-if="streaming && hasThought && !hasMessage"
      class="text-txt3"
      data-testid="gate-busy-status"
    >
      {{ t('pages.clarify.thinkingBusy') }}
    </div>
    <div
      v-else-if="streaming && hasMessage"
      class="gate-stream-outputting"
      data-testid="gate-busy-status"
    >
      {{ t('pages.clarify.outputting') }}
    </div>

    <details
      v-if="hasThought"
      class="rounded border border-line bg-base/60 text-[11.5px] text-txt3"
      data-testid="gate-react-thought"
      :open="thoughtOpen"
      @toggle="onThoughtToggle"
    >
      <summary class="flex cursor-pointer select-none items-center gap-1 px-2 py-1 hover:text-txt2">
        <Icon name="sparkles" :size="11" class="text-accent-2" />
        {{ t('pages.clarify.thought') }}
      </summary>
      <div class="whitespace-pre-wrap border-t border-dashed border-line px-2 pb-1.5 pt-1 font-mono leading-5">
        {{ streamThought }}
      </div>
    </details>

    <div v-if="hasMessage" data-testid="gate-react-message">
      {{ streamText }}
      <span
        v-if="streaming"
        class="gate-stream-caret"
        data-testid="gate-stream-caret"
        aria-hidden="true"
      />
      <span v-if="interrupted" class="ml-1 text-[10px] text-warn">interrupted</span>
    </div>

    <div
      v-if="showCompleted"
      class="flex items-center justify-between gap-2 border-t border-line pt-1.5 text-[11px] text-txt3"
      data-testid="gate-turn-completed"
    >
      <span class="text-txt2">{{ t('pages.clarify.turnCompleted') }}</span>
      <span>{{ locale && completedAt ? relTime(completedAt) : '' }}</span>
    </div>
  </div>
</template>

<style scoped>
.typing-dots {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.typing-dots i {
  width: 5px;
  height: 5px;
  border-radius: 9999px;
  background: #22d3ee;
  animation: typing-bounce 1.2s infinite ease-in-out both;
}
.typing-dots i:nth-child(2) {
  animation-delay: 0.16s;
}
.typing-dots i:nth-child(3) {
  animation-delay: 0.32s;
}
@keyframes typing-bounce {
  0%,
  70%,
  100% {
    transform: translateY(0);
    opacity: 0.35;
  }
  35% {
    transform: translateY(-4px);
    opacity: 1;
  }
}
.gate-stream-outputting {
  color: rgb(var(--c-accent-2));
  background: var(--grad-logo);
  background-size: var(--grad-logo-size);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  animation: shimmer 3.5s ease-in-out infinite;
}
.gate-stream-caret {
  display: inline-block;
  width: 7px;
  height: 1em;
  margin-left: 2px;
  vertical-align: text-bottom;
  background: rgb(var(--c-accent));
  animation: gate-caret-blink 0.9s step-end infinite;
}
@keyframes gate-caret-blink {
  50% {
    opacity: 0;
  }
}
</style>

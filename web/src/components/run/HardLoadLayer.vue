<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    /** Stuck-warning threshold in ms. Short REST 10s / preview 20s / boot·start 60–120s. */
    stuckAfterMs?: number
    /** Stage label; defaults to “Loading”. */
    stage?: string
    /** Overlay (absolute inset) vs inline block. */
    overlay?: boolean
    /** Show retry when stuck warning is visible. */
    showRetry?: boolean
  }>(),
  {
    stuckAfterMs: 10_000,
    overlay: true,
    showRetry: true,
  },
)

const emit = defineEmits<{ retry: [] }>()

const { t } = useI18n()

const elapsedSec = ref(0)
const stuck = ref(false)
let timer: number | undefined
let startedAt = 0

function tick() {
  elapsedSec.value = Math.max(0, Math.floor((Date.now() - startedAt) / 1000))
  if (!stuck.value && Date.now() - startedAt >= props.stuckAfterMs) {
    stuck.value = true
  }
}

function startClock() {
  stopClock()
  startedAt = Date.now()
  elapsedSec.value = 0
  stuck.value = false
  tick()
  timer = window.setInterval(tick, 1000)
}

function stopClock() {
  if (timer != null) {
    clearInterval(timer)
    timer = undefined
  }
}

function onRetry() {
  startClock()
  emit('retry')
}

const stageLabel = computed(() => props.stage || t('common.loading.label'))

onMounted(startClock)
onBeforeUnmount(stopClock)
watch(
  () => props.stuckAfterMs,
  () => {
    startClock()
  },
)

defineExpose({ reset: startClock, elapsedSec, stuck })
</script>

<template>
  <div
    class="flex flex-col items-center justify-center gap-2.5 px-4 py-8 text-center"
    :class="overlay ? 'absolute inset-0 z-[2] bg-base/92' : 'min-h-[200px]'"
    role="status"
    aria-live="polite"
    aria-busy="true"
    data-testid="hard-load-layer"
  >
    <span class="inline-flex h-3 items-end gap-[3px]" aria-hidden="true" data-testid="hard-load-heartbeat">
      <i class="hard-hb inline-block w-[3px] bg-accent" style="height: 6px; animation-delay: 0s" />
      <i class="hard-hb inline-block w-[3px] bg-accent" style="height: 8px; animation-delay: 0.15s" />
      <i class="hard-hb inline-block w-[3px] bg-accent" style="height: 12px; animation-delay: 0.3s" />
    </span>
    <p class="text-[13px] font-semibold text-txt" data-testid="hard-load-stage">{{ stageLabel }}</p>
    <p class="text-[12px] text-txt3" data-testid="hard-load-elapsed">
      {{ t('common.loading.elapsed', { s: elapsedSec }) }}
    </p>
    <div
      v-if="stuck"
      class="max-w-[360px] border border-warn/40 bg-warn/10 px-2.5 py-2 text-[12px] text-warn"
      data-testid="hard-load-stuck"
    >
      {{ t('common.loading.stuck') }}
    </div>
    <button
      v-if="stuck && showRetry"
      type="button"
      class="mt-1 inline-flex min-h-11 min-w-[44px] items-center justify-center border border-line bg-surface px-3 text-[12px] font-medium text-txt hover:bg-elevated"
      data-testid="hard-load-retry"
      @click="onRetry"
    >
      {{ t('common.chatImage.retry') }}
    </button>
  </div>
</template>

<style scoped>
.hard-hb {
  animation: hard-hb 1s ease-in-out infinite;
  transform-origin: bottom;
}
@keyframes hard-hb {
  0%,
  100% {
    opacity: 0.35;
    transform: scaleY(0.6);
  }
  50% {
    opacity: 1;
    transform: scaleY(1);
  }
}
</style>

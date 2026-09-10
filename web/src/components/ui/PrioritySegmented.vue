<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

export type RunPriority = 'high' | 'normal' | 'low'

const props = withDefaults(
  defineProps<{
    modelValue: RunPriority
    disabled?: boolean
  }>(),
  { disabled: false },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: RunPriority): void }>()
const { t } = useI18n()

const options = computed(() =>
  (['high', 'normal', 'low'] as RunPriority[]).map((value) => ({
    value,
    label: t(`common.priority.${value}`),
  })),
)

const track = ref<HTMLElement | null>(null)
const indicatorStyle = ref<Record<string, string>>({
  opacity: '0',
  transform: 'translateX(0)',
  width: '0px',
})

function select(value: RunPriority) {
  if (props.disabled || value === props.modelValue) return
  emit('update:modelValue', value)
}

function btnClass(value: RunPriority) {
  const active = props.modelValue === value
  if (!active) return 'text-txt2 hover:bg-elevated/60 hover:text-txt'
  if (value === 'high') return 'text-err'
  if (value === 'low') return 'text-txt3'
  return 'text-accent-2'
}

function indicatorTone() {
  if (props.modelValue === 'high') return 'bg-err/15'
  if (props.modelValue === 'low') return 'bg-elevated'
  return 'bg-accent-dim'
}

function updateIndicator() {
  const root = track.value
  if (!root) return
  const active = root.querySelector<HTMLElement>('[data-seg-active="true"]')
  if (!active) {
    indicatorStyle.value = { opacity: '0', transform: 'translateX(0)', width: '0px' }
    return
  }
  indicatorStyle.value = {
    opacity: '1',
    transform: `translateX(${active.offsetLeft}px)`,
    width: `${active.offsetWidth}px`,
  }
}

watch(
  () => props.modelValue,
  () => {
    void nextTick(updateIndicator)
  },
)

onMounted(() => {
  void nextTick(updateIndicator)
  window.addEventListener('resize', updateIndicator)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', updateIndicator)
})
</script>

<template>
  <div
    ref="track"
    class="relative grid grid-cols-3 overflow-hidden rounded-md border border-line bg-base"
    role="radiogroup"
    :aria-label="t('common.priority.label')"
    :aria-disabled="disabled || undefined"
  >
    <span
      class="seg-indicator pointer-events-none absolute inset-y-0 left-0 rounded-[5px]"
      :class="indicatorTone()"
      data-testid="priority-seg-indicator"
      :style="indicatorStyle"
      aria-hidden="true"
    />
    <button
      v-for="(opt, i) in options"
      :key="opt.value"
      type="button"
      role="radio"
      :aria-checked="modelValue === opt.value"
      :disabled="disabled"
      class="relative z-[1] px-2 py-2 text-[13px] font-medium transition disabled:cursor-not-allowed disabled:opacity-45"
      :class="[btnClass(opt.value), i < options.length - 1 ? 'border-r border-line' : '']"
      :data-seg-active="modelValue === opt.value ? 'true' : undefined"
      @click="select(opt.value)"
    >
      {{ opt.label }}
    </button>
  </div>
</template>

<style scoped>
.seg-indicator {
  transition:
    transform var(--dur-ui) var(--ease-out-expo),
    width var(--dur-ui) var(--ease-out-expo),
    background-color var(--dur-ui) ease,
    opacity var(--dur-ui) ease;
}
</style>

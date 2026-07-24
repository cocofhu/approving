<script setup lang="ts">
import { computed } from 'vue'
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

function select(value: RunPriority) {
  if (props.disabled || value === props.modelValue) return
  emit('update:modelValue', value)
}

function btnClass(value: RunPriority) {
  const active = props.modelValue === value
  if (!active) return 'text-txt2 hover:bg-elevated hover:text-txt'
  if (value === 'high') return 'bg-err/15 text-err'
  if (value === 'low') return 'bg-elevated text-txt3'
  return 'bg-accent-dim text-accent-2'
}
</script>

<template>
  <div
    class="grid grid-cols-3 border border-line bg-base"
    role="radiogroup"
    :aria-label="t('common.priority.label')"
    :aria-disabled="disabled || undefined"
  >
    <button
      v-for="(opt, i) in options"
      :key="opt.value"
      type="button"
      role="radio"
      :aria-checked="modelValue === opt.value"
      :disabled="disabled"
      class="px-2 py-2 text-[13px] font-medium transition disabled:cursor-not-allowed disabled:opacity-45"
      :class="[btnClass(opt.value), i < options.length - 1 ? 'border-r border-line' : '']"
      @click="select(opt.value)"
    >
      {{ opt.label }}
    </button>
  </div>
</template>

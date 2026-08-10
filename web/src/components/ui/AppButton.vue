<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import AppSpinner from './AppSpinner.vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'ghost' | 'outline' | 'danger' | 'subtle'
    size?: 'sm' | 'md'
    icon?: string
    block?: boolean
    disabled?: boolean
    loading?: boolean
  }>(),
  { variant: 'outline', size: 'md', disabled: false, loading: false },
)

const cls = computed(() => {
  const base =
    'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition outline-none disabled:opacity-50 disabled:cursor-not-allowed'
  const sizes = props.size === 'sm' ? 'px-2.5 py-1 text-xs' : 'px-3.5 py-2 text-sm'
  const variants: Record<string, string> = {
    primary: 'bg-accent text-white hover:bg-accent-2 shadow-glow',
    ghost: 'text-txt2 hover:bg-elevated hover:text-txt',
    outline: 'border border-line bg-surface text-txt hover:border-line-strong hover:bg-elevated',
    danger: 'border border-err/40 bg-err/10 text-err hover:bg-err/20',
    subtle: 'bg-elevated text-txt2 hover:text-txt',
  }
  return [base, sizes, variants[props.variant], props.block ? 'w-full' : '']
})

const isDisabled = computed(() => props.disabled || props.loading)
</script>

<template>
  <button :class="cls" :disabled="isDisabled" :aria-busy="loading ? 'true' : undefined">
    <AppSpinner v-if="loading" :size="size === 'sm' ? 12 : 14" />
    <Icon v-else-if="icon" :name="icon" :size="size === 'sm' ? 14 : 16" />
    <slot />
  </button>
</template>

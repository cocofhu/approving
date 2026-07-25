<script setup lang="ts">
/**
 * Square accent switch (role=switch) — shared boolean control.
 * On: bg-accent track + white square thumb on the right (attachment style).
 * Off: border-line-strong / bg-base + thumb on the left (Pm enable semantics).
 */
const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    disabled?: boolean
  }>(),
  { modelValue: false, disabled: false },
)

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

function toggle() {
  if (props.disabled) return
  emit('update:modelValue', !props.modelValue)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    toggle()
  }
}
</script>

<template>
  <button
    type="button"
    role="switch"
    class="relative h-[22px] w-10 shrink-0 border transition disabled:cursor-not-allowed disabled:opacity-45"
    :class="modelValue ? 'border-accent bg-accent' : 'border-line-strong bg-base'"
    :aria-checked="modelValue"
    :aria-disabled="disabled ? 'true' : undefined"
    :disabled="disabled"
    :tabindex="disabled ? -1 : 0"
    @click="toggle"
    @keydown="onKeydown"
  >
    <span
      class="pointer-events-none absolute top-0.5 h-4 w-4 transition-all"
      :class="modelValue ? 'left-[18px] bg-white' : 'left-0.5 bg-txt2'"
    />
  </button>
</template>

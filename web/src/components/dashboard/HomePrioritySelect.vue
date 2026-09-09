<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RunPriority } from '@/components/ui/PrioritySegmented.vue'

const PRIORITIES: RunPriority[] = ['high', 'normal', 'low']
const PANEL_WIDTH = 168

const props = withDefaults(
  defineProps<{
    modelValue: RunPriority
    disabled?: boolean
  }>(),
  { disabled: false },
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: RunPriority): void
}>()

const { t } = useI18n()

const open = ref(false)
const activeIndex = ref(0)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const panel = ref<HTMLElement | null>(null)
/** Fixed position below trigger (Teleport escapes .home-composer overflow-hidden). */
const panelStyle = ref<Record<string, string>>({})

const selectedLabel = computed(() => t(`common.priority.${props.modelValue}`))

function placePanelBelow() {
  const trig = trigger.value
  if (!trig) return
  const r = trig.getBoundingClientRect()
  const width = Math.min(PANEL_WIDTH, window.innerWidth - 16)
  let left = r.left
  left = Math.max(8, Math.min(left, window.innerWidth - width - 8))
  const top = r.bottom + 6
  panelStyle.value = {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    width: `${Math.round(width)}px`,
    zIndex: '60',
  }
}

function onScrollOrResize() {
  if (open.value) placePanelBelow()
}

async function openPanel() {
  if (props.disabled) return
  open.value = true
  activeIndex.value = Math.max(0, PRIORITIES.indexOf(props.modelValue))
  await nextTick()
  placePanelBelow()
}

function closePanel() {
  open.value = false
  panelStyle.value = {}
}

function togglePanel(e: MouseEvent) {
  e.stopPropagation()
  if (props.disabled) return
  if (open.value) closePanel()
  else void openPanel()
}

function choose(value: RunPriority) {
  emit('update:modelValue', value)
  closePanel()
  trigger.value?.focus()
}

function onDocClick(e: MouseEvent) {
  if (!open.value) return
  const target = e.target as Node
  if (root.value?.contains(target) || panel.value?.contains(target)) return
  closePanel()
}

function onDocKeydown(e: KeyboardEvent) {
  if (!open.value) return
  if (e.key === 'Escape') {
    e.preventDefault()
    closePanel()
    trigger.value?.focus()
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeIndex.value = (activeIndex.value + 1) % PRIORITIES.length
    return
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = (activeIndex.value - 1 + PRIORITIES.length) % PRIORITIES.length
    return
  }
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    choose(PRIORITIES[activeIndex.value])
  }
}

function onTriggerKeydown(e: KeyboardEvent) {
  if (props.disabled) return
  if (open.value) return
  if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    void openPanel()
  }
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onDocKeydown)
  window.addEventListener('resize', onScrollOrResize)
  window.addEventListener('scroll', onScrollOrResize, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onDocKeydown)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize, true)
})
</script>

<template>
  <div
    ref="root"
    class="home-priority-select"
    data-testid="home-priority-select"
    :class="{ 'home-priority-select--open': open }"
  >
    <button
      ref="trigger"
      id="home-priority-select"
      type="button"
      class="home-priority-select__trigger"
      :class="{
        'home-priority-select__trigger--high': modelValue === 'high',
        'home-priority-select__trigger--low': modelValue === 'low',
      }"
      data-testid="home-priority-select-trigger"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-label="t('common.priority.label')"
      aria-controls="home-priority-select-panel"
      @click="togglePanel"
      @keydown="onTriggerKeydown"
    >
      <span class="home-priority-select__label">{{ selectedLabel }}</span>
      <span class="home-priority-select__chev" aria-hidden="true" />
    </button>

    <!-- Teleport to body so .home-composer overflow-hidden cannot clip the panel (plan g1.2) -->
    <Teleport to="body">
      <div
        v-if="open"
        ref="panel"
        id="home-priority-select-panel"
        class="home-priority-select__panel"
        role="listbox"
        data-testid="home-priority-select-panel"
        data-placement="below"
        :aria-label="t('common.priority.label')"
        :style="panelStyle"
        @click.stop
      >
        <button
          v-for="(value, i) in PRIORITIES"
          :key="value"
          type="button"
          class="home-priority-select__opt"
          role="option"
          :class="{
            'home-priority-select__opt--current': value === modelValue,
            'home-priority-select__opt--active': i === activeIndex,
          }"
          :aria-selected="value === modelValue"
          :data-testid="`home-priority-select-option-${value}`"
          @click.stop="choose(value)"
        >
          {{ t(`common.priority.${value}`) }}
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.home-priority-select {
  position: relative;
  max-width: 7.5rem;
  min-width: 0;
  flex-shrink: 0;
}

.home-priority-select__trigger {
  width: 100%;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border: 1px solid rgb(var(--c-line));
  border-radius: 8px;
  background: transparent;
  color: rgb(var(--c-txt));
  padding: 0 10px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;
}

.home-priority-select__trigger:hover:not(:disabled),
.home-priority-select--open .home-priority-select__trigger {
  border-color: rgb(var(--c-line-strong));
}

.home-priority-select__trigger:disabled {
  cursor: default;
  color: rgb(var(--c-txt3));
}

.home-priority-select__trigger--high:not(:disabled) {
  color: rgb(var(--c-err));
  border-color: rgb(var(--c-err) / 0.35);
  background: rgb(var(--c-err) / 0.08);
}

.home-priority-select__trigger--low:not(:disabled) {
  color: rgb(var(--c-txt3));
}

.home-priority-select__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.home-priority-select__chev {
  width: 0;
  height: 0;
  border: 4px solid transparent;
  border-top-color: rgb(var(--c-txt3));
  flex-shrink: 0;
}

.home-priority-select__panel {
  position: fixed;
  left: 0;
  top: calc(100% + 6px);
  width: 168px;
  border: 1px solid rgb(var(--c-line-strong));
  border-radius: 12px;
  overflow: hidden;
  background: rgb(var(--c-elevated));
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45);
  z-index: 60;
  padding: 4px;
}

.home-priority-select__opt {
  width: 100%;
  text-align: left;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: rgb(var(--c-txt));
  padding: 8px 10px;
  font-size: 13px;
  cursor: pointer;
}

.home-priority-select__opt:hover,
.home-priority-select__opt--active {
  background: rgb(var(--c-accent) / 0.16);
  color: rgb(var(--c-txt));
}

.home-priority-select__opt--current {
  color: rgb(var(--c-accent-2));
  font-weight: 600;
}

@media (max-width: 520px) {
  .home-priority-select {
    max-width: 6.5rem;
  }
}
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

export type ProjectAgentOption = { name: string }

const props = withDefaults(
  defineProps<{
    agents: ProjectAgentOption[]
    modelValue: string
    disabled?: boolean
  }>(),
  { disabled: false },
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
}>()

const { t } = useI18n()

const open = ref(false)
const search = ref('')
const activeIndex = ref(0)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)

const selectedName = computed(() => {
  if (!props.modelValue) return t('pages.projectDetail.sharedAgent.pickAgentPlaceholder')
  return props.modelValue
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return props.agents
  return props.agents.filter((a) => a.name.toLowerCase().includes(q))
})

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) =>
  ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[c] as string)
}

function highlightName(name: string, q: string): string {
  if (!q) return escapeHtml(name)
  const lower = name.toLowerCase()
  const iq = q.toLowerCase()
  const i = lower.indexOf(iq)
  if (i < 0) return escapeHtml(name)
  return (
    escapeHtml(name.slice(0, i)) +
    '<mark>' +
    escapeHtml(name.slice(i, i + q.length)) +
    '</mark>' +
    escapeHtml(name.slice(i + q.length))
  )
}

function openPanel() {
  if (props.disabled || !props.agents.length) return
  open.value = true
  search.value = ''
  const idx = props.agents.findIndex((a) => a.name === props.modelValue)
  activeIndex.value = Math.max(0, idx)
  nextTick(() => searchInput.value?.focus())
}

function closePanel() {
  open.value = false
}

function togglePanel(e: MouseEvent) {
  e.stopPropagation()
  if (props.disabled || !props.agents.length) return
  if (open.value) closePanel()
  else openPanel()
}

function choose(name: string) {
  emit('update:modelValue', name)
  closePanel()
  trigger.value?.focus()
}

function onDocClick(e: MouseEvent) {
  if (!open.value) return
  const target = e.target as Node
  if (root.value?.contains(target)) return
  closePanel()
}

function onSearchInput() {
  activeIndex.value = 0
}

function onSearchKeydown(e: KeyboardEvent) {
  const items = filtered.value
  if (e.key === 'Escape') {
    e.preventDefault()
    closePanel()
    trigger.value?.focus()
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (!items.length) return
    activeIndex.value = (activeIndex.value + 1) % items.length
    return
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (!items.length) return
    activeIndex.value = (activeIndex.value - 1 + items.length) % items.length
    return
  }
  if (e.key === 'Enter') {
    e.preventDefault()
    if (!items.length) return
    choose(items[activeIndex.value].name)
  }
}

function onTriggerKeydown(e: KeyboardEvent) {
  if (props.disabled || !props.agents.length) return
  if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    if (!open.value) openPanel()
  }
  if (e.key === 'Escape' && open.value) {
    e.preventDefault()
    closePanel()
  }
}

watch(
  () => filtered.value.length,
  (len) => {
    if (activeIndex.value >= len) activeIndex.value = Math.max(0, len - 1)
  },
)

onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>

<template>
  <div
    ref="root"
    class="project-agent-select"
    data-testid="project-agent-select"
    :class="{ 'project-agent-select--open': open }"
  >
    <button
      ref="trigger"
      type="button"
      class="project-agent-select__trigger"
      data-testid="project-agent-select-trigger"
      :disabled="disabled || !agents.length"
      aria-haspopup="listbox"
      :aria-expanded="open"
      aria-controls="project-agent-select-panel"
      @click="togglePanel"
      @keydown="onTriggerKeydown"
    >
      <span class="project-agent-select__label">{{ selectedName }}</span>
      <span class="project-agent-select__chev" aria-hidden="true" />
    </button>

    <div
      v-if="open"
      id="project-agent-select-panel"
      class="project-agent-select__panel"
      role="presentation"
      data-testid="project-agent-select-panel"
    >
      <div class="project-agent-select__search-wrap">
        <input
          ref="searchInput"
          v-model="search"
          type="search"
          class="project-agent-select__search"
          data-testid="project-agent-select-search"
          :placeholder="t('common.search.agentPlaceholder')"
          autocomplete="off"
          :aria-label="t('common.search.agentPlaceholder')"
          @input="onSearchInput"
          @keydown="onSearchKeydown"
          @click.stop
        />
      </div>
      <div
        class="project-agent-select__list"
        role="listbox"
        :aria-label="t('pages.projectDetail.sharedAgent.pickAgent')"
      >
        <template v-if="filtered.length">
          <button
            v-for="(a, i) in filtered"
            :key="a.name"
            type="button"
            class="project-agent-select__opt"
            role="option"
            :class="{
              'project-agent-select__opt--current': a.name === modelValue,
              'project-agent-select__opt--active': i === activeIndex,
            }"
            :aria-selected="i === activeIndex"
            :data-testid="`project-agent-select-option-${a.name}`"
            @click.stop="choose(a.name)"
          >
            <span v-html="highlightName(a.name, search.trim())" />
          </button>
        </template>
        <div
          v-else
          class="project-agent-select__empty"
          data-testid="project-agent-select-empty"
        >
          {{ t('common.empty.noMatchingAgents') }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.project-agent-select {
  position: relative;
  max-width: 28rem;
  min-width: 12rem;
}

.project-agent-select__trigger {
  width: 100%;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-base));
  color: rgb(var(--c-txt));
  padding: 0 10px;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease;
}

.project-agent-select__trigger:hover:not(:disabled),
.project-agent-select--open .project-agent-select__trigger {
  border-color: rgb(var(--c-line-strong));
}

.project-agent-select__trigger:disabled {
  cursor: default;
  color: rgb(var(--c-txt3));
}

.project-agent-select__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.project-agent-select__chev {
  width: 0;
  height: 0;
  border: 4px solid transparent;
  border-top-color: rgb(var(--c-txt3));
  flex-shrink: 0;
}

.project-agent-select__panel {
  position: absolute;
  left: 0;
  top: calc(100% + 6px);
  width: min(320px, 78vw);
  border: 1px solid rgb(var(--c-line-strong));
  background: rgb(var(--c-elevated));
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45);
  z-index: 20;
}

.project-agent-select__search-wrap {
  padding: 8px;
  border-bottom: 1px solid rgb(var(--c-line) / 0.7);
}

.project-agent-select__search {
  width: 100%;
  height: 32px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-surface));
  color: rgb(var(--c-txt));
  padding: 0 10px;
  font-size: 13px;
  outline: none;
}

.project-agent-select__search::placeholder {
  color: rgb(var(--c-txt3));
}

.project-agent-select__search:focus {
  border-color: rgb(var(--c-accent));
}

.project-agent-select__list {
  max-height: 220px;
  overflow: auto;
  padding: 4px;
}

.project-agent-select__opt {
  width: 100%;
  text-align: left;
  border: 0;
  background: transparent;
  color: rgb(var(--c-txt));
  padding: 8px 10px;
  font-size: 13px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  cursor: pointer;
}

.project-agent-select__opt:hover,
.project-agent-select__opt--active {
  background: rgb(var(--c-accent) / 0.16);
  color: rgb(var(--c-txt));
}

.project-agent-select__opt--current {
  color: rgb(var(--c-accent-2));
}

.project-agent-select__opt :deep(mark) {
  background: rgb(var(--c-accent) / 0.35);
  color: inherit;
  padding: 0 1px;
}

.project-agent-select__empty {
  padding: 16px 10px;
  text-align: center;
  color: rgb(var(--c-txt3));
  font-size: 12px;
}
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

export type HomePipelineOption = { id: string; name: string }

const props = withDefaults(
  defineProps<{
    pipelines: HomePipelineOption[]
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
  if (!props.pipelines.length) return t('pages.dashboard.noPipelineShort')
  const hit = props.pipelines.find((p) => p.id === props.modelValue)
  return hit?.name ?? t('pages.dashboard.noPipelineShort')
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return props.pipelines
  return props.pipelines.filter((p) => p.name.toLowerCase().includes(q))
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
  if (props.disabled || !props.pipelines.length) return
  open.value = true
  search.value = ''
  const idx = props.pipelines.findIndex((p) => p.id === props.modelValue)
  activeIndex.value = Math.max(0, idx)
  nextTick(() => searchInput.value?.focus())
}

function closePanel() {
  open.value = false
}

function togglePanel(e: MouseEvent) {
  e.stopPropagation()
  if (props.disabled || !props.pipelines.length) return
  if (open.value) closePanel()
  else openPanel()
}

function choose(id: string) {
  emit('update:modelValue', id)
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
    choose(items[activeIndex.value].id)
  }
}

function onTriggerKeydown(e: KeyboardEvent) {
  if (props.disabled || !props.pipelines.length) return
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
    class="home-pipeline-select"
    data-testid="home-pipeline-select"
    :class="{ 'home-pipeline-select--open': open }"
  >
    <button
      ref="trigger"
      id="home-pipeline-select"
      type="button"
      class="home-pipeline-select__trigger"
      data-testid="home-pipeline-select-trigger"
      :disabled="disabled || !pipelines.length"
      aria-haspopup="listbox"
      :aria-expanded="open"
      aria-controls="home-pipeline-select-panel"
      @click="togglePanel"
      @keydown="onTriggerKeydown"
    >
      <span class="home-pipeline-select__label">{{ selectedName }}</span>
      <span class="home-pipeline-select__chev" aria-hidden="true" />
    </button>

    <div
      v-if="open"
      id="home-pipeline-select-panel"
      class="home-pipeline-select__panel"
      role="presentation"
      data-testid="home-pipeline-select-panel"
    >
      <div class="home-pipeline-select__search-wrap">
        <input
          ref="searchInput"
          v-model="search"
          type="search"
          class="home-pipeline-select__search"
          data-testid="home-pipeline-select-search"
          :placeholder="t('common.search.pipelinePlaceholder')"
          autocomplete="off"
          :aria-label="t('common.search.pipelinePlaceholder')"
          @input="onSearchInput"
          @keydown="onSearchKeydown"
          @click.stop
        />
      </div>
      <div
        class="home-pipeline-select__list"
        role="listbox"
        :aria-label="t('pages.dashboard.pickPipeline')"
      >
        <template v-if="filtered.length">
          <button
            v-for="(p, i) in filtered"
            :key="p.id"
            type="button"
            class="home-pipeline-select__opt"
            role="option"
            :class="{
              'home-pipeline-select__opt--current': p.id === modelValue,
              'home-pipeline-select__opt--active': i === activeIndex,
            }"
            :aria-selected="i === activeIndex"
            :data-testid="`home-pipeline-select-option-${p.id}`"
            @click.stop="choose(p.id)"
          >
            <span v-html="highlightName(p.name, search.trim())" />
          </button>
        </template>
        <div
          v-else
          class="home-pipeline-select__empty"
          data-testid="home-pipeline-select-empty"
        >
          {{ t('common.empty.noMatchingPipelines') }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.home-pipeline-select {
  position: relative;
  max-width: 14rem;
  min-width: 0;
}

.home-pipeline-select__trigger {
  width: 100%;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border: 1px solid rgb(var(--c-line));
  background: transparent;
  color: rgb(var(--c-accent-2));
  padding: 0 10px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease;
}

.home-pipeline-select__trigger:hover:not(:disabled),
.home-pipeline-select--open .home-pipeline-select__trigger {
  border-color: rgb(var(--c-line-strong));
}

.home-pipeline-select__trigger:disabled {
  cursor: default;
  color: rgb(var(--c-txt3));
}

.home-pipeline-select__label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.home-pipeline-select__chev {
  width: 0;
  height: 0;
  border: 4px solid transparent;
  border-top-color: rgb(var(--c-txt3));
  flex-shrink: 0;
}

.home-pipeline-select__panel {
  position: absolute;
  left: 0;
  bottom: calc(100% + 6px);
  width: min(320px, 78vw);
  border: 1px solid rgb(var(--c-line-strong));
  background: rgb(var(--c-elevated));
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45);
  z-index: 20;
}

.home-pipeline-select__search-wrap {
  padding: 8px;
  border-bottom: 1px solid rgb(var(--c-line) / 0.7);
}

.home-pipeline-select__search {
  width: 100%;
  height: 32px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-surface));
  color: rgb(var(--c-txt));
  padding: 0 10px;
  font-size: 13px;
  outline: none;
}

.home-pipeline-select__search::placeholder {
  color: rgb(var(--c-txt3));
}

.home-pipeline-select__search:focus {
  border-color: rgb(var(--c-accent));
}

.home-pipeline-select__list {
  max-height: 220px;
  overflow: auto;
  padding: 4px;
}

.home-pipeline-select__opt {
  width: 100%;
  text-align: left;
  border: 0;
  background: transparent;
  color: rgb(var(--c-txt));
  padding: 8px 10px;
  font-size: 13px;
  cursor: pointer;
}

.home-pipeline-select__opt:hover,
.home-pipeline-select__opt--active {
  background: rgb(var(--c-accent) / 0.16);
  color: rgb(var(--c-txt));
}

.home-pipeline-select__opt--current {
  color: rgb(var(--c-accent-2));
}

.home-pipeline-select__opt :deep(mark) {
  background: rgb(var(--c-accent) / 0.35);
  color: inherit;
  padding: 0 1px;
}

.home-pipeline-select__empty {
  padding: 16px 10px;
  text-align: center;
  color: rgb(var(--c-txt3));
  font-size: 12px;
}

@media (max-width: 520px) {
  .home-pipeline-select {
    max-width: none;
  }
}
</style>

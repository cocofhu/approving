<script lang="ts">
/**
 * Closes whichever panel is open. Module scope on purpose: instances have to see
 * each other to keep at most one panel open.
 */
let closeOpenPanel: (() => void) | null = null
</script>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: { value: string; label: string; hint?: string }[]
    disabled?: boolean
    invalid?: boolean
    placeholder?: string
    ariaLabel?: string
    /** Adds a filter box on top of the list; matches are highlighted. */
    searchable?: boolean
    searchPlaceholder?: string
    /** Lets the typed query be used verbatim, so the list stays a suggestion. */
    allowCustom?: boolean
    /** Says the options are still on their way, so "no matches" stays unsaid. */
    loading?: boolean
    /** sm matches compact 12px form rows; md matches .input sizing. */
    size?: 'sm' | 'md'
  }>(),
  {
    disabled: false,
    invalid: false,
    searchable: false,
    allowCustom: false,
    loading: false,
    size: 'md',
  },
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
const panel = ref<HTMLElement | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
/** Fixed position: Teleport escapes modal/scroll-area overflow clipping. */
const panelStyle = ref<Record<string, string>>({})

const matched = computed(() => props.options.find((o) => o.value === props.modelValue))
const selectedLabel = computed(() => {
  if (matched.value) return matched.value.label
  // A custom value is its own label; only a truly empty field shows the placeholder.
  if (props.allowCustom && props.modelValue) return props.modelValue
  return props.placeholder || ''
})
const isPlaceholder = computed(
  () => !matched.value && !(props.allowCustom && props.modelValue),
)

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q || !props.searchable) return props.options
  return props.options.filter(
    (o) =>
      o.label.toLowerCase().includes(q) ||
      o.value.toLowerCase().includes(q) ||
      (o.hint || '').toLowerCase().includes(q),
  )
})

/** The typed query as a pickable row, unless it already is an option. */
const customValue = computed(() => {
  if (!props.allowCustom || !props.searchable) return ''
  const q = search.value.trim()
  if (!q) return ''
  return props.options.some((o) => o.value === q) ? '' : q
})

/** Rows the keyboard walks: the custom row, when present, sits first. */
const rows = computed<{ value: string; label: string; hint: string; custom: boolean }[]>(() => [
  ...(customValue.value
    ? [{ value: customValue.value, label: customValue.value, hint: '', custom: true }]
    : []),
  ...filtered.value.map((o) => ({ value: o.value, label: o.label, hint: o.hint || '', custom: false })),
])

function escapeHtml(s: string): string {
  return s.replace(
    /[&<>"']/g,
    (c) =>
      ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      })[c] as string,
  )
}

function highlight(text: string): string {
  const q = search.value.trim()
  if (!q || !props.searchable) return escapeHtml(text)
  const i = text.toLowerCase().indexOf(q.toLowerCase())
  if (i < 0) return escapeHtml(text)
  return (
    escapeHtml(text.slice(0, i)) +
    '<mark>' +
    escapeHtml(text.slice(i, i + q.length)) +
    '</mark>' +
    escapeHtml(text.slice(i + q.length))
  )
}

const MARGIN = 8
const GAP = 6
const MAX_PANEL_HEIGHT = 288

function placePanel() {
  const trig = trigger.value
  if (!trig) return
  const r = trig.getBoundingClientRect()
  const height = Math.min(panel.value?.offsetHeight || MAX_PANEL_HEIGHT, MAX_PANEL_HEIGHT)
  const below = window.innerHeight - r.bottom - GAP - MARGIN
  const flipUp = below < height && r.top - GAP - MARGIN > below
  const top = flipUp ? Math.max(MARGIN, r.top - GAP - height) : r.bottom + GAP
  panelStyle.value = {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    left: `${Math.round(r.left)}px`,
    width: `${Math.round(r.width)}px`,
    maxHeight: `${Math.round(Math.min(MAX_PANEL_HEIGHT, flipUp ? r.top - GAP - MARGIN : below))}px`,
    zIndex: '60',
  }
}

function onScrollOrResize() {
  if (open.value) placePanel()
}

async function openPanel() {
  if (props.disabled) return
  // A trigger click stops propagating, so a parent row or menu does not react to
  // it — which also hides it from every other select's document listener. Panels
  // therefore hand over here, keeping at most one open.
  if (closeOpenPanel && closeOpenPanel !== closePanel) closeOpenPanel()
  closeOpenPanel = closePanel
  open.value = true
  search.value = ''
  activeIndex.value = Math.max(
    0,
    props.options.findIndex((o) => o.value === props.modelValue),
  )
  await nextTick()
  placePanel()
  searchInput.value?.focus()
}

function closePanel() {
  if (closeOpenPanel === closePanel) closeOpenPanel = null
  open.value = false
  search.value = ''
  panelStyle.value = {}
}

function togglePanel(e: MouseEvent) {
  e.stopPropagation()
  if (props.disabled) return
  if (open.value) closePanel()
  else void openPanel()
}

function choose(value: string) {
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

/** Shared by the panel search box and the document-level fallback. */
function handleListKeydown(e: KeyboardEvent): void {
  const items = rows.value
  if (e.key === 'Escape') {
    e.preventDefault()
    closePanel()
    trigger.value?.focus()
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (items.length) activeIndex.value = (activeIndex.value + 1) % items.length
    return
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (items.length) activeIndex.value = (activeIndex.value - 1 + items.length) % items.length
    return
  }
  if (e.key === 'Enter' || (e.key === ' ' && !props.searchable)) {
    e.preventDefault()
    const opt = items[activeIndex.value]
    if (opt) choose(opt.value)
  }
}

function onDocKeydown(e: KeyboardEvent) {
  if (!open.value) return
  // The search box handles its own keys, including space as literal input.
  if (props.searchable && e.target === searchInput.value) return
  handleListKeydown(e)
}

function onTriggerKeydown(e: KeyboardEvent) {
  if (props.disabled || open.value) return
  if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    void openPanel()
  }
}

watch(
  () => rows.value.length,
  (len) => {
    if (activeIndex.value >= len) activeIndex.value = Math.max(0, len - 1)
  },
)

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onDocKeydown)
  window.addEventListener('resize', onScrollOrResize)
  window.addEventListener('scroll', onScrollOrResize, true)
})
onBeforeUnmount(() => {
  if (closeOpenPanel === closePanel) closeOpenPanel = null
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onDocKeydown)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize, true)
})
</script>

<template>
  <div ref="root" class="relative">
    <button
      ref="trigger"
      type="button"
      class="flex w-full items-center gap-2 rounded-md border bg-base px-3 py-2 text-left text-txt transition"
      :class="[
        size === 'sm' ? 'text-[12px]' : 'text-sm',
        invalid ? 'border-err' : open ? 'border-accent' : 'border-line hover:border-line-strong',
        disabled ? 'cursor-default text-txt3' : '',
      ]"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-label="ariaLabel"
      data-test="app-select-trigger"
      @click="togglePanel"
      @keydown="onTriggerKeydown"
    >
      <span class="min-w-0 flex-1 truncate" :class="isPlaceholder ? 'text-txt3' : ''">
        {{ selectedLabel }}
      </span>
      <Icon name="chevron-down" :size="14" class="shrink-0 text-txt3" />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="panel"
        class="card app-select-panel flex flex-col overflow-hidden"
        :aria-label="ariaLabel"
        data-test="app-select-panel"
        :style="panelStyle"
        @click.stop
      >
        <div v-if="searchable" class="shrink-0 border-b border-line/70 p-2">
          <input
            ref="searchInput"
            v-model="search"
            type="search"
            class="w-full rounded-md border border-line bg-surface px-2.5 py-1.5 text-[12px] text-txt outline-none transition placeholder:text-txt3 focus:border-accent"
            :placeholder="searchPlaceholder || t('common.search.optionPlaceholder')"
            :aria-label="searchPlaceholder || t('common.search.optionPlaceholder')"
            autocomplete="off"
            data-test="app-select-search"
            @click.stop
            @input="activeIndex = 0"
            @keydown="handleListKeydown"
          />
        </div>

        <div
          class="scroll-area min-h-0 flex-1 overflow-y-auto p-1"
          role="listbox"
          :aria-label="ariaLabel"
        >
          <button
            v-for="(row, i) in rows"
            :key="(row.custom ? 'custom:' : 'opt:') + row.value"
            type="button"
            role="option"
            class="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left transition"
            :class="[
              size === 'sm' ? 'text-[12px]' : 'text-sm',
              row.value === modelValue ? 'text-accent-2' : 'text-txt2',
              i === activeIndex ? 'bg-accent-dim text-txt' : 'hover:bg-elevated',
            ]"
            :aria-selected="row.value === modelValue"
            :data-test="row.custom ? 'app-select-custom' : `app-select-option-${row.value}`"
            @click.stop="choose(row.value)"
          >
            <span v-if="row.custom" class="min-w-0 flex-1 truncate">
              {{ t('common.search.useTypedValue', { value: row.value }) }}
            </span>
            <span v-else class="min-w-0 flex-1 truncate" v-html="highlight(row.label)" />
            <span
              v-if="row.hint"
              class="min-w-0 shrink-0 truncate text-[11px] text-txt3"
              v-html="highlight(row.hint)"
            />
            <Icon
              v-if="row.value === modelValue"
              name="check"
              :size="14"
              class="shrink-0 text-accent-2"
            />
          </button>
          <p
            v-if="!rows.length"
            class="px-2.5 py-4 text-center text-[12px] text-txt3"
            :data-test="loading ? 'app-select-loading' : 'app-select-empty'"
          >
            {{ loading ? t('common.search.loadingOptions') : t('common.empty.noMatchingOptions') }}
          </p>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.app-select-panel :deep(mark) {
  background: rgb(var(--c-accent) / 0.35);
  color: inherit;
  padding: 0 1px;
}
</style>

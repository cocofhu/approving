<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export type AuditDdOption = {
  value: string
  label: string
  sub?: string
  short?: string
  dot?: string
}

const props = withDefaults(
  defineProps<{
    labelKey?: string
    modelValue: string
    options: AuditDdOption[]
    searchable?: boolean
    emptyLabel?: string
    width?: number
    right?: boolean
    /** Full-width trigger (mobile filter editor); lifts max-width:220px. */
    block?: boolean
    groupBy?: (o: AuditDdOption) => string
    disabled?: boolean
    testId?: string
  }>(),
  {
    labelKey: '',
    searchable: false,
    emptyLabel: '',
    width: 260,
    right: false,
    block: false,
    disabled: false,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const open = ref(false)
const find = ref('')
const trigEl = ref<HTMLButtonElement | null>(null)
const panelEl = ref<HTMLDivElement | null>(null)
const panelStyle = ref<Record<string, string>>({})

const current = computed(() => props.options.find((o) => o.value === props.modelValue) || null)

const displayValue = computed(() => {
  const c = current.value
  if (!c || c.value === '') {
    return c?.label || props.emptyLabel || '全部'
  }
  return c.short || c.label
})

const muted = computed(() => !current.value || current.value.value === '')

const filtered = computed(() => {
  const q = find.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter((o) =>
    `${o.label || ''}${o.value || ''}${o.sub || ''}`.toLowerCase().includes(q),
  )
})

function place() {
  const trig = trigEl.value
  if (!trig) return
  const r = trig.getBoundingClientRect()
  const maxW = Math.max(120, window.innerWidth - 16)
  const pw = props.block
    ? Math.min(Math.max(r.width, 160), maxW)
    : Math.min(props.width, maxW)
  let left = props.right ? r.right - pw : r.left
  left = Math.max(8, Math.min(left, window.innerWidth - pw - 8))
  const top = r.bottom + 6
  const space = window.innerHeight - top - 8
  panelStyle.value = {
    left: `${left}px`,
    top: `${top}px`,
    width: `${pw}px`,
    maxHeight: `${Math.max(160, Math.min(320, space))}px`,
  }
}

function close() {
  open.value = false
  find.value = ''
}

async function toggle(e: MouseEvent) {
  e.stopPropagation()
  if (props.disabled) return
  if (open.value) {
    close()
    return
  }
  open.value = true
  find.value = ''
  await nextTick()
  place()
}

function pick(v: string) {
  emit('update:modelValue', v)
  close()
}

function onDocClick(e: MouseEvent) {
  const t = e.target as Node
  if (trigEl.value?.contains(t) || panelEl.value?.contains(t)) return
  close()
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}

function onScrollOrResize() {
  if (open.value) place()
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
  document.addEventListener('keydown', onKey)
  window.addEventListener('resize', onScrollOrResize)
  window.addEventListener('scroll', onScrollOrResize, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKey)
  window.removeEventListener('resize', onScrollOrResize)
  window.removeEventListener('scroll', onScrollOrResize, true)
})

watch(
  () => props.options,
  () => {
    if (open.value) place()
  },
)

defineExpose({ close })
</script>

<template>
  <div class="audit-dd" :class="{ open, block, 'is-hide': false }" :data-testid="testId">
    <button
      ref="trigEl"
      type="button"
      class="audit-dd-trig"
      :disabled="disabled"
      @click="toggle"
    >
      <span v-if="labelKey" class="k">{{ labelKey }} · </span>
      <span class="v" :class="{ muted }">{{ displayValue }}</span>
      <svg class="caret" viewBox="0 0 12 12" fill="none" aria-hidden="true">
        <path d="M3 4.5L6 7.5L9 4.5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
      </svg>
    </button>
    <Teleport to="body">
      <div
        v-show="open"
        ref="panelEl"
        class="audit-dd-panel"
        :style="panelStyle"
        @click.stop
      >
        <div v-if="searchable" class="audit-dd-find">
          <input v-model="find" type="search" placeholder="筛选…" autocomplete="off" />
        </div>
        <div class="audit-dd-list">
          <template v-if="filtered.length">
            <template v-for="(o, i) in filtered" :key="`${o.value}-${i}`">
              <div
                v-if="groupBy && (i === 0 || groupBy(o) !== groupBy(filtered[i - 1]!))"
                class="audit-dd-group"
              >
                {{ groupBy(o) }}
              </div>
              <button
                type="button"
                class="audit-dd-opt"
                :class="{ on: o.value === modelValue }"
                @click="pick(o.value)"
              >
                <span class="dot" :class="o.dot || ''" />
                <span class="main">
                  <span class="t">{{ o.label }}</span>
                  <span v-if="o.sub" class="s">{{ o.sub }}</span>
                </span>
                <span class="tick">✓</span>
              </button>
            </template>
          </template>
          <div v-else class="audit-dd-empty">无匹配</div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.audit-dd {
  position: relative;
  display: inline-flex;
  flex: 0 0 auto;
  vertical-align: middle;
}
.audit-dd.block {
  display: block;
  width: 100%;
  flex: 1 1 auto;
}
.audit-dd-trig {
  height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 220px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-surface));
  padding: 0 8px 0 10px;
  font: inherit;
  font-size: 12px;
  color: rgb(var(--c-txt));
  cursor: pointer;
  text-align: left;
  white-space: nowrap;
  border-radius: 0;
}
.audit-dd.block .audit-dd-trig {
  display: flex;
  width: 100%;
  max-width: none;
  min-height: 44px;
  height: auto;
  padding: 10px 12px;
  white-space: normal;
}
.audit-dd-trig:hover:not(:disabled) {
  border-color: rgb(var(--c-line-strong));
  background: rgb(var(--c-elevated));
}
.audit-dd.open .audit-dd-trig {
  border-color: rgb(var(--c-accent));
  box-shadow: 0 0 0 3px rgb(var(--c-accent) / 0.12);
}
.audit-dd-trig:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.audit-dd-trig .k {
  color: rgb(var(--c-txt2));
  font-weight: 500;
  flex: 0 0 auto;
}
.audit-dd-trig .v {
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 500;
  max-width: 140px;
}
.audit-dd.block .audit-dd-trig .v {
  flex: 1 1 auto;
  min-width: 0;
  max-width: none;
}
.audit-dd-trig .v.muted {
  color: rgb(var(--c-txt2));
  font-weight: 400;
}
.caret {
  width: 12px;
  height: 12px;
  color: rgb(var(--c-txt2));
  flex: 0 0 auto;
  margin-left: auto;
  transition: transform 0.12s;
}
.audit-dd.open .caret {
  transform: rotate(180deg);
  color: rgb(var(--c-accent));
}
</style>

<style>
.audit-dd-panel {
  position: fixed;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  max-width: calc(100vw - 24px);
  background: #fff;
  border: 1px solid #e4e4e7;
  box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.06), 0 12px 32px rgba(0, 0, 0, 0.12);
  border-radius: 0;
}
.audit-dd-find {
  padding: 8px;
  border-bottom: 1px solid #ececef;
  flex: 0 0 auto;
}
.audit-dd-find input {
  width: 100%;
  height: 30px;
  border: 1px solid #e4e4e7;
  padding: 0 9px;
  font: inherit;
  font-size: 12px;
  outline: none;
  background: #fafafa;
  border-radius: 0;
}
.audit-dd-find input:focus {
  border-color: #c4b5fd;
  background: #fff;
}
.audit-dd-list {
  overflow: auto;
  padding: 4px;
  flex: 1 1 auto;
  min-height: 0;
}
.audit-dd-group {
  padding: 8px 8px 4px;
  font-size: 10px;
  font-weight: 650;
  color: #a1a1aa;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.audit-dd-opt {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: 0;
  background: transparent;
  padding: 7px 8px;
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
  color: #18181b;
  border-radius: 0;
}
.audit-dd-opt:hover {
  background: #f4f4f5;
}
.audit-dd-opt.on {
  background: #f5f3ff;
  color: #7c3aed;
}
.audit-dd-opt .dot {
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  background: #d4d4d8;
}
.audit-dd-opt .dot.mcp {
  background: #8b5cf6;
}
.audit-dd-opt .dot.run {
  background: #71717a;
}
.audit-dd-opt .dot.gate {
  background: #d97706;
}
.audit-dd-opt .dot.wf {
  background: #2563eb;
}
.audit-dd-opt .dot.prj {
  background: #16a34a;
}
.audit-dd-opt .dot.aud {
  background: #db2777;
}
.audit-dd-opt .main {
  flex: 1;
  min-width: 0;
}
.audit-dd-opt .main .t {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}
.audit-dd-opt .main .s {
  display: block;
  font-size: 11px;
  color: #71717a;
  margin-top: 1px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.audit-dd-opt .tick {
  width: 14px;
  opacity: 0;
  color: #7c3aed;
  font-size: 12px;
  font-weight: 700;
}
.audit-dd-opt.on .tick {
  opacity: 1;
}
.audit-dd-empty {
  padding: 20px 12px;
  text-align: center;
  color: #a1a1aa;
  font-size: 12px;
}
</style>

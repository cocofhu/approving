<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'

const props = withDefaults(
  defineProps<{
    page: number
    pageSize: number
    total: number
    loading?: boolean
    disabled?: boolean
    /** When provided (non-empty), render per-page elevated select. */
    pageSizeOptions?: number[]
    /** Optional summary text; omit to keep common.pagination.rangeSummary. */
    summaryOverride?: string
    summaryTestId?: string
    pageSizeTestId?: string
  }>(),
  {
    loading: false,
    disabled: false,
    pageSizeOptions: undefined,
    summaryOverride: undefined,
    summaryTestId: undefined,
    pageSizeTestId: undefined,
  },
)

const emit = defineEmits<{
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()

const { t } = useI18n()
const { isMobile } = useBreakpoint()

const controlsDisabled = computed(() => props.loading || props.disabled)

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize) || 1))

const rangeFrom = computed(() => {
  if (props.total <= 0) return 0
  return (props.page - 1) * props.pageSize + 1
})

const rangeTo = computed(() => {
  if (props.total <= 0) return 0
  return Math.min(props.page * props.pageSize, props.total)
})

const summaryText = computed(() => {
  if (props.summaryOverride !== undefined) return props.summaryOverride
  if (props.total <= 0) return t('common.pagination.emptySummary')
  return t('common.pagination.rangeSummary', {
    from: rangeFrom.value,
    to: rangeTo.value,
    total: props.total,
  })
})

const pageNumbers = computed(() => {
  const tp = totalPages.value
  const p = props.page
  let start = Math.max(1, p - 2)
  const end = Math.min(tp, start + 4)
  start = Math.max(1, end - 4)
  const nums: number[] = []
  for (let i = start; i <= end; i++) nums.push(i)
  return nums
})

const showPageSize = computed(
  () => Array.isArray(props.pageSizeOptions) && props.pageSizeOptions.length > 0,
)

const atStart = computed(() => props.page <= 1 || props.total <= 0)
const atEnd = computed(() => props.page >= totalPages.value || props.total <= 0)

function goTo(p: number) {
  if (controlsDisabled.value) return
  if (p < 1 || p > totalPages.value || p === props.page) return
  emit('update:page', p)
}

function onPageSizeChange(event: Event) {
  if (controlsDisabled.value) return
  const raw = (event.target as HTMLSelectElement).value
  const next = Number(raw)
  if (!Number.isFinite(next) || next <= 0 || next === props.pageSize) return
  emit('update:pageSize', next)
}
</script>

<template>
  <nav
    class="pagination"
    :class="{ 'is-mobile': isMobile }"
    :aria-busy="loading ? 'true' : undefined"
  >
    <div class="pg-summary" :data-testid="summaryTestId || undefined">
      <slot name="summary">{{ summaryText }}</slot>
    </div>
    <div class="pg-nav">
      <button
        type="button"
        class="pg-btn"
        :disabled="atStart || controlsDisabled"
        :aria-label="t('common.pagination.prev')"
        @click="goTo(page - 1)"
      >
        <Icon name="chevron-left" :size="14" />
        <span>{{ t('common.pagination.prev') }}</span>
      </button>
      <div v-if="!isMobile" class="page-nums">
        <button
          v-for="p in pageNumbers"
          :key="p"
          type="button"
          class="page-num"
          :class="{ active: p === page }"
          :disabled="controlsDisabled"
          :aria-current="p === page ? 'page' : undefined"
          :aria-label="String(p)"
          @click="goTo(p)"
        >
          {{ p }}
        </button>
      </div>
      <button
        type="button"
        class="pg-btn"
        :disabled="atEnd || controlsDisabled"
        :aria-label="t('common.pagination.next')"
        @click="goTo(page + 1)"
      >
        <span>{{ t('common.pagination.next') }}</span>
        <Icon name="chevron-right" :size="14" />
      </button>
    </div>
    <div v-if="showPageSize" class="pg-size" :data-testid="pageSizeTestId || undefined">
      <label class="pg-size-label">
        <span>{{ t('common.pagination.perPage') }}</span>
        <select
          class="pg-size-select"
          :value="pageSize"
          :disabled="controlsDisabled"
          @change="onPageSizeChange"
        >
          <option v-for="opt in pageSizeOptions" :key="opt" :value="opt">{{ opt }}</option>
        </select>
      </label>
    </div>
  </nav>
</template>

<style scoped>
.pagination {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 12px;
  padding: 12px 14px;
  border-top: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-surface));
  font-size: 13px;
  color: rgb(var(--c-txt2));
}
.pg-summary {
  font-size: 12px;
  color: rgb(var(--c-txt2));
  min-width: 96px;
  font-variant-numeric: tabular-nums;
}
.pg-nav {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.pg-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-elevated));
  color: rgb(var(--c-txt));
  padding: 6px 10px;
  font-size: 12px;
  transition: border-color 0.12s, color 0.12s, background 0.12s;
}
.pg-btn:hover:not(:disabled) {
  border-color: rgb(var(--c-line-strong));
  background: rgb(var(--c-overlay));
}
.pg-btn:disabled {
  color: rgb(var(--c-txt2));
  opacity: 1;
  cursor: not-allowed;
}
.page-nums {
  display: flex;
  gap: 4px;
}
.page-num {
  min-width: 32px;
  height: 32px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-elevated));
  color: rgb(var(--c-txt2));
  font-size: 12px;
  padding: 0 8px;
  transition: border-color 0.12s, color 0.12s, background 0.12s;
}
.page-num.active {
  color: rgb(var(--c-accent-2));
  background: rgb(var(--c-accent-dim));
  border-color: #7b61ff;
  box-shadow: inset 0 0 0 1px rgba(123, 97, 255, 0.35);
  font-weight: 600;
}
.page-num:hover:not(:disabled):not(.active) {
  border-color: rgb(var(--c-line-strong));
  color: rgb(var(--c-txt));
}
.page-num:disabled {
  color: rgb(var(--c-txt2));
  opacity: 1;
  cursor: not-allowed;
}
.pg-size {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: rgb(var(--c-txt2));
}
.pg-size-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.pg-size-select {
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-elevated));
  color: rgb(var(--c-txt));
  padding: 6px 8px;
  font-size: 12px;
}
.pg-size-select:disabled {
  color: rgb(var(--c-txt2));
  opacity: 1;
  cursor: not-allowed;
}
</style>

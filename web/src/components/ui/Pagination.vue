<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBreakpoint } from '@/lib/useBreakpoint'

const props = defineProps<{
  page: number
  pageSize: number
  total: number
}>()

const emit = defineEmits<{
  'update:page': [page: number]
}>()

const { t } = useI18n()
const { isMobile } = useBreakpoint()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))

const pageNumbers = computed(() => {
  const tp = totalPages.value
  const p = props.page
  const start = Math.max(1, p - 2)
  const end = Math.min(tp, start + 4)
  const nums: number[] = []
  for (let i = start; i <= end; i++) nums.push(i)
  return nums
})

function goTo(p: number) {
  if (p < 1 || p > totalPages.value || p === props.page) return
  emit('update:page', p)
}
</script>

<template>
  <div
    class="border-t border-line px-5 py-3 text-[13px] text-txt2"
    :class="isMobile ? 'flex flex-col gap-2' : 'flex items-center justify-between'"
  >
    <span>
      {{ t('common.pagination.summary', { total, page, pages: totalPages }) }}
    </span>
    <div class="flex items-center gap-1">
      <button
        type="button"
        class="pager-btn"
        :disabled="page <= 1"
        @click="goTo(page - 1)"
      >
        {{ t('common.pagination.prev') }}
      </button>
      <template v-if="!isMobile">
        <button
          v-for="p in pageNumbers"
          :key="p"
          type="button"
          class="pager-btn page-num"
          :class="{ active: p === page }"
          @click="goTo(p)"
        >
          {{ p }}
        </button>
      </template>
      <button
        type="button"
        class="pager-btn"
        :disabled="page >= totalPages"
        @click="goTo(page + 1)"
      >
        {{ t('common.pagination.next') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.pager-btn {
  padding: 6px 12px;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-elevated));
  color: rgb(var(--c-txt2));
  font-size: 12px;
  transition: border-color 0.12s, color 0.12s, background 0.12s;
}
.pager-btn:hover:not(:disabled) {
  border-color: rgb(var(--c-line-strong));
  color: rgb(var(--c-txt));
}
.pager-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.pager-btn.page-num {
  min-width: 32px;
  text-align: center;
  padding: 6px 8px;
}
.pager-btn.page-num.active {
  background: rgb(var(--c-accent-dim));
  border-color: rgb(var(--c-accent));
  color: rgb(var(--c-txt));
  font-weight: 600;
}
</style>

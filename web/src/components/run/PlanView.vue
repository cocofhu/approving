<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import AnnotateBtn from './product/AnnotateBtn.vue'

export type PlanSub = { id?: string; title?: string; detail?: string; status?: string }
export type PlanGoal = { id?: string; title?: string; detail?: string; status?: string; subgoals?: PlanSub[] }
export type PlanDoc = { title?: string; goals?: PlanGoal[] }

const props = defineProps<{ doc: PlanDoc; accent?: string }>()

const { t } = useI18n()
const hex = computed(() => props.accent || '#818CF8')

const progress = computed(() => {
  const goals = props.doc.goals || []
  const leaves: PlanSub[] = []
  for (const g of goals) {
    if (g.subgoals?.length) leaves.push(...g.subgoals)
    else leaves.push(g)
  }
  const done = leaves.filter((l) => l.status === 'done').length
  return { done, total: leaves.length, pct: leaves.length ? Math.round((done / leaves.length) * 100) : 0 }
})

const STATUS: Record<string, { labelKey: string; cls: string; dot: string }> = {
  done: { labelKey: 'pages.plan.status.done', cls: 'bg-ok/15 text-ok', dot: 'bg-ok' },
  in_progress: { labelKey: 'pages.plan.status.in_progress', cls: 'bg-info/15 text-info', dot: 'bg-info animate-pulseglow' },
  pending: { labelKey: 'pages.plan.status.pending', cls: 'bg-elevated text-txt3', dot: 'bg-line-strong' },
}
function st(s?: string) {
  const key = s || 'pending'
  const meta = STATUS[key] || STATUS.pending
  return { label: t(meta.labelKey), cls: meta.cls, dot: meta.dot }
}
</script>

<template>
  <div>
    <div class="mb-3 flex items-center gap-2">
      <div class="flex h-7 w-7 items-center justify-center rounded-md" :style="{ background: hex + '22', color: hex }">
        <Icon name="check" :size="15" />
      </div>
      <div class="group min-w-0 flex-1">
        <div class="flex min-w-0 items-center gap-1">
          <div class="truncate text-sm font-semibold text-txt">{{ doc.title || t('pages.plan.defaultTitle') }}</div>
          <AnnotateBtn v-if="doc.title" json-path="title" :label="doc.title" />
        </div>
      </div>
      <span class="shrink-0 rounded-full bg-base px-2 py-0.5 text-[11px] font-medium text-txt3">
        {{ progress.done }}/{{ progress.total }} · {{ progress.pct }}%
      </span>
    </div>

    <!-- overall progress bar -->
    <div class="mb-3 h-1.5 w-full overflow-hidden rounded-full bg-base">
      <div class="h-full rounded-full transition-all" :style="{ width: progress.pct + '%', background: hex }" />
    </div>

    <div class="space-y-2.5">
      <div v-for="(g, gi) in doc.goals || []" :key="g.id || gi" class="rounded-lg border border-line bg-base/40 p-3">
        <div class="flex items-start gap-2.5">
          <span class="mt-1.5 h-2 w-2 shrink-0 rounded-full" :class="st(g.status).dot" />
          <div class="min-w-0 flex-1">
            <div class="group flex flex-wrap items-center gap-2">
              <code class="shrink-0 rounded bg-base px-1.5 py-0.5 font-mono text-[10px] text-txt3">{{ g.id }}</code>
              <span class="text-[13px] font-semibold text-txt">{{ g.title }}</span>
              <AnnotateBtn :json-path="`goals[${g.id || gi}]`" :label="g.title || `目标 ${gi + 1}`" />
              <span class="ml-auto shrink-0 rounded-md px-2 py-0.5 text-[10px] font-medium" :class="st(g.status).cls">{{ st(g.status).label }}</span>
            </div>
            <div v-if="g.detail" class="mt-1 text-[12px] leading-relaxed text-txt3">{{ g.detail }}</div>
          </div>
        </div>

        <div v-if="g.subgoals?.length" class="mt-2.5 space-y-2 border-l-2 border-line pl-3.5">
          <div v-for="(s, si) in g.subgoals" :key="s.id || si" class="flex items-start gap-2.5">
            <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" :class="st(s.status).dot" />
            <div class="min-w-0 flex-1">
              <div class="group flex flex-wrap items-center gap-2">
                <code class="shrink-0 rounded bg-base px-1.5 py-0.5 font-mono text-[10px] text-txt3">{{ s.id }}</code>
                <span class="text-[12px] text-txt2">{{ s.title }}</span>
                <AnnotateBtn :json-path="`goals[${g.id || gi}].subgoals[${s.id || si}]`" :label="s.title || `小目标 ${si + 1}`" />
                <span class="ml-auto shrink-0 rounded-md px-2 py-0.5 text-[10px] font-medium" :class="st(s.status).cls">{{ st(s.status).label }}</span>
              </div>
              <div v-if="s.detail" class="mt-1 text-[11px] leading-relaxed text-txt3">{{ s.detail }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

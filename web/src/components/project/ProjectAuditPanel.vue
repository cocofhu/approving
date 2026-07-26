<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { api, isPaginated, type PaginatedResponse } from '@/lib/api'
import { prettyAuditPayload } from '@/lib/auditPayload'
import { useToast } from '@/lib/useToast'
import { fmtTime } from '@/lib/format'
import type { ProjectAuditEvent } from '@/lib/types'

const props = defineProps<{
  projectId: string
  /** Test/demo hook: force the Demo「无权查看」state without a read-only role. */
  forceDenied?: boolean
}>()

const { t } = useI18n()
const toast = useToast()

const loading = ref(false)
const denied = ref(false)
const events = ref<ProjectAuditEvent[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const openId = ref<string | null>(null)

const timeWindow = ref<'24h' | '7d' | 'all'>('24h')
const actor = ref('')
const action = ref('')
const resource = ref('')

const hasMore = computed(() => page.value * pageSize < total.value)

function buildParams(extra?: { page?: number }) {
  return {
    time: timeWindow.value,
    actor: actor.value || undefined,
    action: action.value || undefined,
    resource: resource.value.trim() || undefined,
    page: extra?.page ?? page.value,
    pageSize,
  }
}

async function load(resetPage = false) {
  if (props.forceDenied) {
    denied.value = true
    events.value = []
    total.value = 0
    return
  }
  if (resetPage) page.value = 1
  loading.value = true
  denied.value = false
  try {
    const res = await api.listProjectAudit(props.projectId, buildParams())
    const pageData: PaginatedResponse<ProjectAuditEvent> = isPaginated(res)
      ? res
      : { items: res as ProjectAuditEvent[], total: (res as ProjectAuditEvent[]).length, page: 1, pageSize, hasMore: false }
    events.value = pageData.items || []
    total.value = pageData.total
    page.value = pageData.page
  } catch (e: any) {
    if (e?.status === 403) {
      denied.value = true
      events.value = []
      total.value = 0
      return
    }
    toast.error(e?.message || t('pages.projectDetail.audit.loadFailed'))
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  openId.value = null
  void load(true)
}

function toggleOpen(id: string) {
  openId.value = openId.value === id ? null : id
}

function actorLabel(ev: ProjectAuditEvent) {
  if (ev.unattributable || ev.actor === 'system') {
    return t('pages.projectDetail.audit.actorSystem')
  }
  return ev.actor
}

function outcomeLabel(ev: ProjectAuditEvent) {
  return ev.outcome === 'fail'
    ? t('pages.projectDetail.audit.outcomeFail')
    : t('pages.projectDetail.audit.outcomeOk')
}

function prettyPayload(payload: Record<string, unknown> | undefined) {
  // Escape + highlight via shared helper (never inject raw JSON into v-html).
  return prettyAuditPayload(payload)
}

async function exportAudit(format: 'json' | 'text') {
  if (denied.value || props.forceDenied) {
    toast.error(t('pages.projectDetail.audit.exportDenied'))
    return
  }
  try {
    const url = api.exportProjectAuditUrl(props.projectId, {
      format,
      time: timeWindow.value,
      actor: actor.value || undefined,
      action: action.value || undefined,
      resource: resource.value.trim() || undefined,
    })
    const a = document.createElement('a')
    a.href = url
    a.download = ''
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    toast.success(t('pages.projectDetail.audit.exportStarted', { format: format.toUpperCase() }))
    // Refresh so meta-audit export event appears.
    setTimeout(() => void load(true), 400)
  } catch (e: any) {
    toast.error(e?.message || t('pages.projectDetail.audit.exportFailed'))
  }
}

function prevPage() {
  if (page.value <= 1) return
  page.value -= 1
  void load()
}

function nextPage() {
  if (!hasMore.value) return
  page.value += 1
  void load()
}

watch(
  () => [props.projectId, props.forceDenied],
  () => void load(true),
)

onMounted(() => void load(true))
</script>

<template>
  <div class="flex min-h-[420px] flex-col" data-testid="project-audit-panel">
    <div
      v-if="denied || forceDenied"
      class="flex flex-1 flex-col items-center justify-center gap-2 border border-dashed border-line bg-surface px-6 py-16 text-center"
      data-testid="project-audit-denied"
    >
      <div class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.audit.deniedTitle') }}</div>
      <p class="max-w-md text-[13px] text-txt3">{{ t('pages.projectDetail.audit.deniedDesc') }}</p>
    </div>

    <template v-else>
      <div
        class="mb-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-[repeat(4,minmax(0,1fr))_auto]"
        data-testid="project-audit-filters"
      >
        <label class="grid gap-1 text-[11px] font-semibold text-txt3">
          {{ t('pages.projectDetail.audit.filterTime') }}
          <select v-model="timeWindow" class="rounded border border-line bg-card px-2.5 py-2 text-[13px] text-txt">
            <option value="24h">{{ t('pages.projectDetail.audit.time24h') }}</option>
            <option value="7d">{{ t('pages.projectDetail.audit.time7d') }}</option>
            <option value="all">{{ t('pages.projectDetail.audit.timeAll') }}</option>
          </select>
        </label>
        <label class="grid gap-1 text-[11px] font-semibold text-txt3">
          {{ t('pages.projectDetail.audit.filterActor') }}
          <input
            v-model="actor"
            type="text"
            class="rounded border border-line bg-card px-2.5 py-2 text-[13px] text-txt"
            :placeholder="t('pages.projectDetail.audit.actorPlaceholder')"
          />
        </label>
        <label class="grid gap-1 text-[11px] font-semibold text-txt3">
          {{ t('pages.projectDetail.audit.filterAction') }}
          <select v-model="action" class="rounded border border-line bg-card px-2.5 py-2 text-[13px] text-txt">
            <option value="">{{ t('pages.projectDetail.audit.actionAll') }}</option>
            <option value="project.config">{{ t('pages.projectDetail.audit.actionProjectConfig') }}</option>
            <option value="workflow">{{ t('pages.projectDetail.audit.actionWorkflow') }}</option>
            <option value="run">{{ t('pages.projectDetail.audit.actionRun') }}</option>
            <option value="gate">{{ t('pages.projectDetail.audit.actionGate') }}</option>
            <option value="mcp">{{ t('pages.projectDetail.audit.actionMcp') }}</option>
            <option value="audit.export">{{ t('pages.projectDetail.audit.actionExport') }}</option>
          </select>
        </label>
        <label class="grid gap-1 text-[11px] font-semibold text-txt3">
          {{ t('pages.projectDetail.audit.filterResource') }}
          <input
            v-model="resource"
            type="text"
            class="rounded border border-line bg-card px-2.5 py-2 text-[13px] text-txt"
            :placeholder="t('pages.projectDetail.audit.resourcePlaceholder')"
          />
        </label>
        <div class="flex flex-wrap items-end gap-2">
          <AppButton size="sm" variant="primary" data-testid="project-audit-apply" @click="applyFilters">
            {{ t('pages.projectDetail.audit.applyFilters') }}
          </AppButton>
          <AppButton size="sm" data-testid="project-audit-export-json" @click="exportAudit('json')">
            {{ t('pages.projectDetail.audit.exportJson') }}
          </AppButton>
          <AppButton size="sm" data-testid="project-audit-export-text" @click="exportAudit('text')">
            {{ t('pages.projectDetail.audit.exportText') }}
          </AppButton>
        </div>
      </div>

      <div class="mb-2 flex items-center justify-between text-[12px] text-txt3">
        <span data-testid="project-audit-count">
          {{ t('pages.projectDetail.audit.resultCount', { n: total }) }}
        </span>
        <span>{{ t('pages.projectDetail.audit.expandHint') }}</span>
      </div>

      <div v-if="loading" class="py-10 text-center text-[13px] text-txt3">{{ t('pages.projectDetail.audit.loading') }}</div>
      <EmptyState
        v-else-if="!events.length"
        data-testid="project-audit-empty"
        :title="t('pages.projectDetail.audit.emptyTitle')"
        :desc="t('pages.projectDetail.audit.emptyDesc')"
      />
      <div v-else class="grid gap-2" data-testid="project-audit-list">
        <article
          v-for="ev in events"
          :key="ev.id"
          class="cursor-pointer overflow-hidden rounded-lg border border-line bg-card transition hover:border-accent/40"
          :class="openId === ev.id ? 'border-accent shadow-[0_0_0_3px_rgba(13,122,111,.12)]' : ''"
          :data-testid="`project-audit-event-${ev.id}`"
          @click="toggleOpen(ev.id)"
        >
          <div class="grid gap-2 px-3.5 py-3 sm:grid-cols-[150px_1fr_auto] sm:items-start">
            <div class="font-mono text-[12px] text-txt3">{{ fmtTime(ev.occurredAt) }}</div>
            <div>
              <p class="m-0 text-[13.5px] font-semibold text-txt">{{ ev.summary || ev.action }}</p>
              <div class="mt-1 flex flex-wrap gap-1.5 text-[12px] text-txt2">
                <span
                  class="inline-flex rounded-full border px-2 py-0.5"
                  :class="
                    ev.unattributable
                      ? 'border-line bg-surface text-txt3'
                      : 'border-emerald-200 bg-emerald-50 text-emerald-800'
                  "
                >
                  {{ actorLabel(ev) }}
                </span>
                <span class="inline-flex rounded-full border border-line bg-surface px-2 py-0.5">{{ ev.action }}</span>
                <span
                  class="inline-flex rounded-full border border-line bg-surface px-2 py-0.5"
                  :class="ev.outcome === 'fail' ? 'text-red-700' : 'text-emerald-700'"
                >
                  {{ outcomeLabel(ev) }}
                </span>
                <span class="inline-flex rounded-full border border-line bg-surface px-2 py-0.5">
                  {{ ev.resource || `${ev.resourceType}/${ev.resourceId}` }}
                </span>
              </div>
            </div>
            <div class="text-right text-[12px] text-txt3">{{ ev.summary }}</div>
          </div>
          <div v-if="openId === ev.id" class="border-t border-line bg-surface px-3.5 pb-3.5 pt-3" @click.stop>
            <div class="grid gap-3 sm:grid-cols-2">
              <dl class="text-[12.5px]">
                <dt class="font-semibold text-txt3">{{ t('pages.projectDetail.audit.fieldTime') }}</dt>
                <dd class="mb-2 text-txt">{{ fmtTime(ev.occurredAt) }}</dd>
                <dt class="font-semibold text-txt3">{{ t('pages.projectDetail.audit.fieldActor') }}</dt>
                <dd class="mb-2 text-txt">{{ actorLabel(ev) }}</dd>
                <dt class="font-semibold text-txt3">{{ t('pages.projectDetail.audit.fieldAction') }}</dt>
                <dd class="mb-2 text-txt">{{ ev.action }}</dd>
              </dl>
              <dl class="text-[12.5px]">
                <dt class="font-semibold text-txt3">{{ t('pages.projectDetail.audit.fieldResource') }}</dt>
                <dd class="mb-2 break-all text-txt">{{ ev.resource || `${ev.resourceType}/${ev.resourceId}` }}</dd>
                <dt class="font-semibold text-txt3">{{ t('pages.projectDetail.audit.fieldOutcome') }}</dt>
                <dd class="mb-2 text-txt">{{ outcomeLabel(ev) }}</dd>
                <dt class="font-semibold text-txt3">{{ t('pages.projectDetail.audit.fieldSummary') }}</dt>
                <dd class="mb-2 text-txt">{{ ev.summary }}</dd>
              </dl>
            </div>
            <pre
              class="mt-2 overflow-x-auto rounded-lg bg-[#1a2332] p-3 font-mono text-[11.5px] leading-relaxed text-[#d7e2f0]"
              data-testid="project-audit-payload"
              v-html="prettyPayload(ev.payload)"
            />
          </div>
        </article>
      </div>

      <div class="mt-3 flex items-center justify-between text-[12.5px] text-txt3">
        <span>{{ t('pages.projectDetail.audit.pager', { page, pageSize }) }}</span>
        <div class="flex gap-1.5">
          <AppButton size="sm" variant="ghost" :disabled="page <= 1 || loading" @click="prevPage">
            {{ t('common.pagination.prev') }}
          </AppButton>
          <AppButton size="sm" variant="ghost" :disabled="!hasMore || loading" @click="nextPage">
            {{ t('common.pagination.next') }}
          </AppButton>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
:deep(.tok-key) {
  color: #7dd3c7;
}
:deep(.audit-mask) {
  color: #fbbf24;
}
</style>

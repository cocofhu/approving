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

type FacetResource = { resourceType: string; resourceId: string; resource: string }

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

const actorOptions = ref<string[]>([])
const resourceOptions = ref<FacetResource[]>([])

const hasMore = computed(() => page.value * pageSize < total.value)
const resourceDisabled = computed(() => !action.value)

const actionLabelMap = computed<Record<string, string>>(() => ({
  'project.config': t('pages.projectDetail.audit.actionProjectConfig'),
  workflow: t('pages.projectDetail.audit.actionWorkflow'),
  run: t('pages.projectDetail.audit.actionRun'),
  gate: t('pages.projectDetail.audit.actionGate'),
  mcp: t('pages.projectDetail.audit.actionMcp'),
  'audit.export': t('pages.projectDetail.audit.actionExport'),
}))

const tipTypeText = computed(() =>
  action.value
    ? t('pages.projectDetail.audit.tipTypeReady')
    : t('pages.projectDetail.audit.tipTypeIdle'),
)

const tipResText = computed(() => {
  if (!action.value) return t('pages.projectDetail.audit.tipResLocked')
  if (resource.value) return t('pages.projectDetail.audit.tipResPicked')
  return t('pages.projectDetail.audit.tipResReady')
})

const tipTypeClass = computed(() =>
  action.value ? 'text-emerald-700 font-semibold' : 'text-txt3 font-medium',
)

const tipResClass = computed(() => {
  if (resource.value) return 'text-emerald-700 font-semibold'
  if (action.value) return 'text-accent-2 font-semibold'
  return 'text-txt3 font-medium'
})

const chipParts = computed(() => {
  const parts: string[] = []
  if (action.value) {
    parts.push(actionLabelMap.value[action.value] || action.value)
  }
  if (resource.value) {
    const hit = resourceOptions.value.find((r) => resourceValue(r) === resource.value)
    parts.push(hit ? formatResourceOption(hit) : resource.value)
  }
  if (actor.value) {
    parts.push(actor.value === 'system' ? t('pages.projectDetail.audit.actorSystem') : actor.value)
  }
  return parts
})

const showChip = computed(() => chipParts.value.length > 0)

function resourceValue(r: FacetResource) {
  return r.resourceId || r.resource || r.resourceType
}

function resourceTypeLabel(type: string) {
  const keyMap: Record<string, string> = {
    run: 'resourceLabelRun',
    mcp: 'resourceLabelMcp',
    workflow: 'resourceLabelWorkflow',
    gate: 'resourceLabelGate',
    project: 'resourceLabelProject',
    audit: 'resourceLabelAudit',
    cron: 'resourceLabelCron',
    channel: 'resourceLabelChannel',
    pm: 'resourceLabelPm',
  }
  const key = keyMap[type]
  return key
    ? t(`pages.projectDetail.audit.${key}`)
    : t('pages.projectDetail.audit.resourceLabelGeneric')
}

function formatResourceOption(r: FacetResource) {
  const id = r.resourceId || r.resource
  const prefix = resourceTypeLabel(r.resourceType)
  return id ? `${prefix} · ${id}` : prefix
}

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

async function loadFacets() {
  if (props.forceDenied || denied.value) {
    actorOptions.value = []
    resourceOptions.value = []
    return
  }
  try {
    const res = await api.listProjectAuditFacets(props.projectId, {
      time: timeWindow.value,
      action: action.value || undefined,
    })
    actorOptions.value = Array.isArray(res.actors) ? res.actors : []
    resourceOptions.value = Array.isArray(res.resources) ? res.resources : []
    if (actor.value && !actorOptions.value.includes(actor.value)) {
      actor.value = ''
    }
    if (resource.value && !resourceOptions.value.some((r) => resourceValue(r) === resource.value)) {
      resource.value = ''
    }
  } catch (e: any) {
    if (e?.status === 403) {
      denied.value = true
      actorOptions.value = []
      resourceOptions.value = []
      return
    }
    // Keep prior options on soft failure; list load will surface hard errors.
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
  void loadFacets().then(() => load(true))
}

async function onActionChange() {
  resource.value = ''
  openId.value = null
  await loadFacets()
  void load(true)
}

function onResourceChange() {
  openId.value = null
  void load(true)
}

function resetFilters() {
  timeWindow.value = '24h'
  actor.value = ''
  action.value = ''
  resource.value = ''
  openId.value = null
  void loadFacets().then(() => load(true))
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
  () => {
    void loadFacets().then(() => load(true))
  },
)

onMounted(() => {
  void loadFacets().then(() => load(true))
})
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
        class="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-accent/25 bg-accent/10 px-2.5 py-2 text-[12px] text-accent-2"
        data-testid="project-audit-cascade-banner"
      >
        <strong>{{ t('pages.projectDetail.audit.cascadeBanner1') }}</strong>
        <span class="opacity-55">→</span>
        <strong>{{ t('pages.projectDetail.audit.cascadeBanner2') }}</strong>
        <span class="opacity-55">→</span>
        <span>{{ t('pages.projectDetail.audit.cascadeBanner3') }}</span>
      </div>

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
          <select
            v-model="actor"
            class="rounded border border-line bg-card px-2.5 py-2 text-[13px] text-txt"
            data-testid="project-audit-actor"
          >
            <option value="">{{ t('pages.projectDetail.audit.actorPlaceholder') }}</option>
            <option v-for="a in actorOptions" :key="a" :value="a">
              {{ a === 'system' ? t('pages.projectDetail.audit.actorSystem') : a }}
            </option>
          </select>
        </label>

        <label class="grid gap-1 text-[11px] font-semibold text-txt3" data-testid="project-audit-action-field">
          <span class="inline-flex items-center gap-1">
            <span
              class="inline-flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold"
              :class="action ? 'bg-accent text-white' : 'bg-accent/20 text-accent-2'"
            >1</span>
            {{ t('pages.projectDetail.audit.filterAction') }}
          </span>
          <select
            v-model="action"
            class="rounded border border-line bg-card px-2.5 py-2 text-[13px] text-txt"
            data-testid="project-audit-action"
            @change="onActionChange"
          >
            <option value="">{{ t('pages.projectDetail.audit.actionAll') }}</option>
            <option value="project.config">{{ t('pages.projectDetail.audit.actionProjectConfig') }}</option>
            <option value="workflow">{{ t('pages.projectDetail.audit.actionWorkflow') }}</option>
            <option value="run">{{ t('pages.projectDetail.audit.actionRun') }}</option>
            <option value="gate">{{ t('pages.projectDetail.audit.actionGate') }}</option>
            <option value="mcp">{{ t('pages.projectDetail.audit.actionMcp') }}</option>
            <option value="audit.export">{{ t('pages.projectDetail.audit.actionExport') }}</option>
          </select>
          <span class="min-h-4 text-[11px]" :class="tipTypeClass">{{ tipTypeText }}</span>
        </label>

        <label class="grid gap-1 text-[11px] font-semibold text-txt3" data-testid="project-audit-resource-field">
          <span class="inline-flex items-center gap-1">
            <span
              class="inline-flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold"
              :class="action ? 'bg-accent text-white' : 'bg-accent/20 text-accent-2'"
            >2</span>
            {{ t('pages.projectDetail.audit.filterResource') }}
          </span>
          <select
            v-model="resource"
            class="rounded border bg-card px-2.5 py-2 text-[13px] text-txt disabled:cursor-not-allowed disabled:border-dashed disabled:bg-surface disabled:text-txt3"
            :class="resourceDisabled ? 'border-line' : 'border-line'"
            data-testid="project-audit-resource"
            :disabled="resourceDisabled"
            @change="onResourceChange"
          >
            <option value="">{{ t('pages.projectDetail.audit.resourcePlaceholder') }}</option>
            <option v-for="r in resourceOptions" :key="resourceValue(r)" :value="resourceValue(r)">
              {{ formatResourceOption(r) }}
            </option>
          </select>
          <span class="min-h-4 text-[11px]" :class="tipResClass">{{ tipResText }}</span>
        </label>

        <div class="flex flex-wrap items-end gap-2">
          <AppButton size="sm" variant="primary" data-testid="project-audit-apply" @click="applyFilters">
            {{ t('pages.projectDetail.audit.applyFilters') }}
          </AppButton>
          <AppButton size="sm" data-testid="project-audit-reset" @click="resetFilters">
            {{ t('pages.projectDetail.audit.resetFilters') }}
          </AppButton>
          <AppButton size="sm" data-testid="project-audit-export-json" @click="exportAudit('json')">
            {{ t('pages.projectDetail.audit.exportJson') }}
          </AppButton>
          <AppButton size="sm" data-testid="project-audit-export-text" @click="exportAudit('text')">
            {{ t('pages.projectDetail.audit.exportText') }}
          </AppButton>
        </div>
      </div>

      <div
        v-if="showChip"
        class="mb-2 inline-flex max-w-full flex-wrap items-center gap-1.5 rounded-lg border border-emerald-200 bg-emerald-50 px-2.5 py-1.5 text-[12px] text-emerald-800"
        data-testid="project-audit-filter-chip"
        role="status"
      >
        <span>{{ t('pages.projectDetail.audit.chipPrefix') }}</span>
        <strong class="font-semibold">{{ chipParts.join(' · ') }}</strong>
      </div>

      <div class="mb-2 flex items-center justify-between text-[12px] text-txt3">
        <span data-testid="project-audit-count">
          {{ t('pages.projectDetail.audit.resultCount', { n: total }) }}
        </span>
        <span data-testid="project-audit-expand-hint">{{ t('pages.projectDetail.audit.expandHint') }}</span>
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

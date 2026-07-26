<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import StatusPill from '@/components/ui/StatusPill.vue'
import { api } from '@/lib/api'
import { fmtTime } from '@/lib/format'
import { useToast } from '@/lib/useToast'
import type { AgentCronJob } from '@/lib/types'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const toast = useToast()

const items = ref<AgentCronJob[]>([])
const loading = ref(true)
const togglingId = ref<string | null>(null)
const deletingId = ref<string | null>(null)

async function load() {
  loading.value = true
  try {
    const res = await api.listProjectCronJobs(props.projectId)
    items.value = res.items || []
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    loading.value = false
  }
}

function scheduleLabel(job: AgentCronJob): string {
  const kind = job.scheduleKind || ''
  const expr = job.scheduleExpr || ''
  if (!kind && !expr) return '—'
  return kind ? `${kind}: ${expr}` : expr
}

async function onDeliverToggle(job: AgentCronJob, checked: boolean) {
  if (togglingId.value) return
  const prev = job.deliverToChannel
  job.deliverToChannel = checked
  togglingId.value = job.id
  try {
    const updated = await api.patchProjectCronJob(props.projectId, job.id, {
      deliverToChannel: checked,
    })
    const idx = items.value.findIndex((j) => j.id === job.id)
    if (idx >= 0) items.value[idx] = { ...items.value[idx], ...updated }
  } catch (e: any) {
    job.deliverToChannel = prev
    toast.error(String(e?.message || e))
  } finally {
    togglingId.value = null
  }
}

async function removeJob(id: string) {
  if (!confirm(t('pages.projectDetail.cron.deleteConfirm'))) return
  deletingId.value = id
  try {
    await api.deleteProjectCronJob(props.projectId, id)
    await load()
    toast.success(t('pages.projectDetail.cron.deleted'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    deletingId.value = null
  }
}

watch(
  () => props.projectId,
  () => void load(),
)
onMounted(() => void load())
</script>

<template>
  <div class="flex min-h-[420px] flex-col gap-4" data-testid="project-cron-jobs-panel">
    <div>
      <h3 class="text-base font-semibold">{{ t('pages.projectDetail.cron.title') }}</h3>
      <p class="mt-1 text-sm text-txt3">{{ t('pages.projectDetail.cron.hint') }}</p>
    </div>

    <div v-if="loading" class="text-sm text-txt3">{{ t('common.buttons.loading') }}</div>
    <EmptyState
      v-else-if="!items.length"
      :title="t('pages.projectDetail.cron.emptyTitle')"
      :desc="t('pages.projectDetail.cron.emptyDesc')"
    />
    <div v-else class="overflow-hidden rounded-lg border border-line">
      <div class="scroll-area overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="bg-elevated text-xs text-txt3">
            <tr>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colName') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colAgent') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colSchedule') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colEnabled') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colDeliver') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colNextRun') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colLastRun') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colStatus') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('pages.projectDetail.cron.colActions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="job in items" :key="job.id" class="border-t border-line">
              <td class="px-3 py-2.5">
                <div class="font-medium text-txt">{{ job.name }}</div>
              </td>
              <td class="px-3 py-2.5 text-txt2">{{ job.agentName || '—' }}</td>
              <td class="px-3 py-2.5 font-mono text-xs text-txt2">{{ scheduleLabel(job) }}</td>
              <td class="px-3 py-2.5">
                <span class="text-txt2">
                  {{ job.enabled ? t('pages.projectDetail.cron.enabledYes') : t('pages.projectDetail.cron.enabledNo') }}
                </span>
              </td>
              <td class="px-3 py-2.5">
                <label
                  class="inline-flex items-center gap-2"
                  :title="t('pages.projectDetail.cron.deliverHint')"
                >
                  <AppSwitch
                    data-testid="cron-deliver-toggle"
                    :model-value="job.deliverToChannel"
                    :disabled="togglingId === job.id || deletingId === job.id"
                    :aria-label="t('pages.projectDetail.cron.deliverLabel')"
                    @update:model-value="onDeliverToggle(job, $event)"
                  />
                  <span class="text-xs text-txt3">{{ t('pages.projectDetail.cron.deliverLabel') }}</span>
                </label>
              </td>
              <td class="px-3 py-2.5 text-txt3">{{ job.nextRunAt ? fmtTime(job.nextRunAt) : '—' }}</td>
              <td class="px-3 py-2.5 text-txt3">{{ job.lastRunAt ? fmtTime(job.lastRunAt) : '—' }}</td>
              <td class="px-3 py-2.5">
                <StatusPill v-if="job.lastStatus" :status="job.lastStatus" size="sm" />
                <span v-else class="text-txt3">—</span>
              </td>
              <td class="px-3 py-2.5 text-right">
                <button
                  type="button"
                  class="text-[11px] text-err disabled:cursor-not-allowed disabled:opacity-40"
                  data-testid="project-cron-delete"
                  :disabled="deletingId === job.id || togglingId === job.id"
                  @click="removeJob(job.id)"
                >
                  {{ t('common.buttons.delete') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

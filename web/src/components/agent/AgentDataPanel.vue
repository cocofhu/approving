<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import { api } from '@/lib/api'
import type { ProjectMemoryItem, ChatThread, ChatMessage, AgentCronJob } from '@/lib/types'
import { fmtTime } from '@/lib/format'
import { useToast } from '@/lib/useToast'

export type DataSubTab = 'memory' | 'context' | 'jobs'

const props = withDefaults(
  defineProps<{ agentName: string; projectName: string; subTab?: DataSubTab }>(),
  { subTab: undefined },
)
const emit = defineEmits<{ 'update:subTab': [value: DataSubTab] }>()

const { t } = useI18n()
const toast = useToast()

const adminRequiredTip = computed(() => t('pages.projectDetail.pm.adminRequired'))

const sub = ref<DataSubTab>(props.subTab || 'memory')

watch(
  () => props.subTab,
  (v) => {
    if (v && v !== sub.value) sub.value = v
  },
)

function setSub(next: DataSubTab) {
  if (sub.value === next) return
  sub.value = next
  emit('update:subTab', next)
}

function permissionMessage(err: unknown): string {
  const msg = String((err as { message?: string })?.message || err || '')
  if (/admin required/i.test(msg)) return adminRequiredTip.value
  return msg
}

const memories = ref<ProjectMemoryItem[]>([])
const memLoading = ref(false)
const memTitle = ref('')
const memContent = ref('')
const memEditingId = ref<string | null>(null)
const memSaving = ref(false)

async function loadMemories() {
  memLoading.value = true
  try {
    const res = await api.listAgentMemories(props.agentName)
    memories.value = res.items || []
  } catch (e: any) {
    toast.error(permissionMessage(e))
  } finally {
    memLoading.value = false
  }
}
function editMemory(m: ProjectMemoryItem) {
  memEditingId.value = m.id
  memTitle.value = m.title
  memContent.value = m.content
}
function resetMemForm() {
  memEditingId.value = null
  memTitle.value = ''
  memContent.value = ''
}
async function saveMemory() {
  if (!memTitle.value.trim()) {
    toast.error(t('pages.agentStudio.data.memory.titleRequired'))
    return
  }
  memSaving.value = true
  try {
    if (memEditingId.value) {
      await api.updateAgentMemory(props.agentName, memEditingId.value, {
        title: memTitle.value,
        content: memContent.value,
      })
    } else {
      await api.upsertAgentMemory(props.agentName, { title: memTitle.value, content: memContent.value })
    }
    resetMemForm()
    await loadMemories()
  } catch (e: any) {
    toast.error(permissionMessage(e))
  } finally {
    memSaving.value = false
  }
}
async function removeMemory(id: string) {
  try {
    await api.deleteAgentMemory(props.agentName, id)
    if (memEditingId.value === id) resetMemForm()
    await loadMemories()
  } catch (e: any) {
    toast.error(permissionMessage(e))
  }
}
async function clearMemories() {
  if (!confirm(t('pages.agentStudio.data.memory.clearConfirm'))) return
  try {
    await api.clearAgentMemories(props.agentName)
    resetMemForm()
    await loadMemories()
  } catch (e: any) {
    toast.error(permissionMessage(e))
  }
}

const threads = ref<ChatThread[]>([])
const threadCounts = ref<Record<string, number>>({})
const ctxLoading = ref(false)
const openThreadId = ref<string | null>(null)
const threadMessages = ref<ChatMessage[]>([])
const msgLoading = ref(false)

async function loadThreads() {
  ctxLoading.value = true
  try {
    const res = await api.listAgentThreads(props.agentName)
    threads.value = res.items || []
    threadCounts.value = res.messageCounts || {}
  } catch (e: any) {
    toast.error(permissionMessage(e))
  } finally {
    ctxLoading.value = false
  }
}
async function openThread(id: string) {
  openThreadId.value = id
  msgLoading.value = true
  try {
    const res = await api.listAgentThreadMessages(props.agentName, id)
    threadMessages.value = res.items || []
  } catch (e: any) {
    toast.error(permissionMessage(e))
  } finally {
    msgLoading.value = false
  }
}
async function removeThread(id: string) {
  if (!confirm(t('pages.agentStudio.data.context.deleteConfirm'))) return
  try {
    await api.deleteAgentThread(props.agentName, id)
    if (openThreadId.value === id) {
      openThreadId.value = null
      threadMessages.value = []
    }
    await loadThreads()
  } catch (e: any) {
    toast.error(permissionMessage(e))
  }
}

const jobs = ref<AgentCronJob[]>([])
const jobsLoading = ref(false)
const jobBusy = ref<string | null>(null)

async function loadJobs() {
  jobsLoading.value = true
  try {
    const res = await api.listAgentCronJobs(props.agentName)
    jobs.value = res.items || []
  } catch (e: any) {
    toast.error(permissionMessage(e))
  } finally {
    jobsLoading.value = false
  }
}
async function patchJob(job: AgentCronJob, body: { enabled?: boolean; deliverToChannel?: boolean }) {
  jobBusy.value = job.id
  try {
    const updated = await api.patchAgentCronJob(props.agentName, job.id, body)
    const idx = jobs.value.findIndex((j) => j.id === job.id)
    if (idx >= 0) jobs.value[idx] = { ...jobs.value[idx], ...updated }
  } catch (e: any) {
    toast.error(permissionMessage(e))
    await loadJobs()
  } finally {
    jobBusy.value = null
  }
}
async function removeJob(id: string) {
  if (!confirm(t('pages.agentStudio.data.jobs.deleteConfirm'))) return
  try {
    await api.deleteAgentCronJob(props.agentName, id)
    await loadJobs()
  } catch (e: any) {
    toast.error(permissionMessage(e))
  }
}

function reload() {
  if (sub.value === 'memory') {
    void loadMemories()
  } else if (sub.value === 'context') {
    void loadThreads()
  } else void loadJobs()
}
watch(() => [props.agentName, sub.value], () => reload())
onMounted(() => reload())
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="border-b border-line px-4 py-3">
      <h3 class="text-sm font-semibold text-txt">{{ t('pages.agentStudio.data.title') }}</h3>
      <p class="mt-0.5 text-[12px] text-txt3">
        {{ t('pages.agentStudio.data.hint', { project: projectName || '—' }) }}
      </p>
      <div class="mt-3 flex gap-1">
        <button
          v-for="k in (['memory', 'context', 'jobs'] as const)"
          :key="k"
          type="button"
          class="rounded px-2.5 py-1 text-[12px]"
          :class="sub === k ? 'bg-accent-dim text-accent-2' : 'text-txt3 hover:bg-elevated hover:text-txt'"
          @click="setSub(k)"
        >
          {{ t(`pages.agentStudio.data.tabs.${k}`) }}
        </button>
      </div>
    </div>

    <div class="scroll-area min-h-0 flex-1 overflow-auto p-4">
      <!-- memory: any authenticated user -->
      <div v-if="sub === 'memory'" class="space-y-4">
        <div class="rounded border border-line bg-base p-3">
          <div class="grid gap-2">
            <input v-model="memTitle" class="rounded border border-line bg-surface px-3 py-2 text-sm" :placeholder="t('pages.agentStudio.data.memory.titlePh')" />
            <textarea v-model="memContent" rows="3" class="rounded border border-line bg-surface px-3 py-2 text-sm" :placeholder="t('pages.agentStudio.data.memory.contentPh')" />
            <div class="flex gap-2">
              <button type="button" class="rounded bg-accent px-3 py-1.5 text-[12px] text-white disabled:opacity-50" :disabled="memSaving" @click="saveMemory">
                {{ memEditingId ? t('common.buttons.save') : t('pages.agentStudio.data.memory.add') }}
              </button>
              <button v-if="memEditingId" type="button" class="rounded border border-line px-3 py-1.5 text-[12px]" @click="resetMemForm">{{ t('common.buttons.cancel') }}</button>
              <button v-if="memories.length" type="button" class="ml-auto text-[12px] text-err" @click="clearMemories">{{ t('pages.agentStudio.data.memory.clear') }}</button>
            </div>
          </div>
        </div>
        <div v-if="memLoading" class="text-sm text-txt3">{{ t('common.buttons.loading') }}</div>
        <div v-else-if="!memories.length" class="text-sm text-txt3">{{ t('pages.agentStudio.data.memory.empty') }}</div>
        <div v-else class="space-y-2">
          <div v-for="m in memories" :key="m.id" class="rounded border border-line bg-base p-3">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="truncate text-[13px] font-medium">{{ m.title }}</div>
                <p class="mt-1 whitespace-pre-wrap text-[12px] text-txt2">{{ m.content }}</p>
                <p v-if="m.updatedBy" class="mt-1 text-[11px] text-txt3">
                  {{ t('pages.agentStudio.data.memory.updatedBy', { user: m.updatedBy }) }}
                </p>
              </div>
              <div class="flex shrink-0 gap-1">
                <button type="button" class="rounded border border-line px-2 py-1 text-[11px]" @click="editMemory(m)">{{ t('common.buttons.edit') }}</button>
                <button type="button" class="rounded border border-err/30 px-2 py-1 text-[11px] text-err" @click="removeMemory(m.id)">{{ t('common.buttons.delete') }}</button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- context: any authenticated user -->
      <div v-else-if="sub === 'context'" class="space-y-3">
        <p class="text-[12px] text-txt3">{{ t('pages.agentStudio.data.context.hint') }}</p>
        <div v-if="ctxLoading" class="text-sm text-txt3">{{ t('common.buttons.loading') }}</div>
        <div v-else-if="!threads.length" class="text-sm text-txt3">{{ t('pages.agentStudio.data.context.empty') }}</div>
        <div v-else class="space-y-2">
          <div v-for="th in threads" :key="th.id" class="rounded border border-line bg-base">
            <div class="flex items-center gap-2 px-3 py-2">
              <button type="button" class="min-w-0 flex-1 text-left" @click="openThread(th.id)">
                <div class="truncate text-[13px] font-medium">{{ th.title || th.id }}</div>
                <div class="text-[11px] text-txt3">{{ th.kind || 'user' }} · {{ threadCounts[th.id] || 0 }} msgs · {{ fmtTime(th.updatedAt) }}</div>
              </button>
              <button type="button" class="rounded border border-err/30 px-2 py-1 text-[11px] text-err" @click="removeThread(th.id)">
                <Icon name="trash" :size="12" />
              </button>
            </div>
            <div v-if="openThreadId === th.id" class="border-t border-line px-3 py-2">
              <div v-if="msgLoading" class="text-[12px] text-txt3">{{ t('common.buttons.loading') }}</div>
              <div v-else class="max-h-64 space-y-2 overflow-auto">
                <div v-for="msg in threadMessages" :key="msg.id" class="text-[12px]">
                  <span class="font-medium text-txt2">{{ msg.role }}:</span>
                  <span class="ml-1 whitespace-pre-wrap text-txt">{{ msg.content }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- jobs: list + write for any authenticated user -->
      <div v-else class="space-y-3">
        <p class="text-[12px] text-txt3">
          {{ t('pages.agentStudio.data.jobs.hint') }}
        </p>
        <div v-if="jobsLoading" class="text-sm text-txt3">{{ t('common.buttons.loading') }}</div>
        <div v-else-if="!jobs.length" class="text-sm text-txt3">{{ t('pages.agentStudio.data.jobs.empty') }}</div>
        <table v-else class="w-full text-left text-sm">
          <thead class="text-xs text-txt3">
            <tr>
              <th class="px-2 py-1.5">{{ t('pages.agentStudio.data.jobs.colName') }}</th>
              <th class="px-2 py-1.5">{{ t('pages.agentStudio.data.jobs.colSchedule') }}</th>
              <th class="px-2 py-1.5">{{ t('pages.agentStudio.data.jobs.colEnabled') }}</th>
              <th class="px-2 py-1.5">{{ t('pages.agentStudio.data.jobs.colDeliver') }}</th>
              <th class="px-2 py-1.5" />
            </tr>
          </thead>
          <tbody>
            <tr v-for="job in jobs" :key="job.id" class="border-t border-line">
              <td class="px-2 py-2">{{ job.name }}</td>
              <td class="px-2 py-2 text-txt2">{{ job.scheduleKind }}: {{ job.scheduleExpr }}</td>
              <td class="px-2 py-2">
                <input
                  type="checkbox"
                  data-testid="agent-cron-enabled"
                  :checked="job.enabled"
                  :disabled="jobBusy === job.id"
                  @change="patchJob(job, { enabled: ($event.target as HTMLInputElement).checked })"
                />
              </td>
              <td class="px-2 py-2">
                <input
                  type="checkbox"
                  data-testid="agent-cron-deliver"
                  :checked="job.deliverToChannel"
                  :disabled="jobBusy === job.id"
                  @change="patchJob(job, { deliverToChannel: ($event.target as HTMLInputElement).checked })"
                />
              </td>
              <td class="px-2 py-2 text-right">
                <button
                  type="button"
                  class="text-[11px] text-err disabled:cursor-not-allowed disabled:opacity-40"
                  data-testid="agent-cron-delete"
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

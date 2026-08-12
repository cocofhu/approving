<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import Icon from '@/components/ui/Icon.vue'
import { api, type ChannelConfig, type ChannelConfigInput } from '@/lib/api/api'
import {
  deriveRecentPushTargets,
  pushTargetPrimaryLabel,
  type PushTargetOption,
} from '@/lib/pm/cronDeliverTargets'
import { useToast } from '@/lib/composables/useToast'
import type { Project, ProjectNotifyPolicy } from '@/lib/shared/types'

const props = defineProps<{
  projectId: string
  project: Project
  pmLeaderAgent?: string
}>()

const emit = defineEmits<{
  'project-updated': [project: Project]
}>()

const { t } = useI18n()
const toast = useToast()

const PM_MCP_OPTIONS = [
  { id: 'pm-progress', labelKey: 'pages.projectDetail.pm.mcpProgress' },
  { id: 'pm-workflow-read', labelKey: 'pages.projectDetail.pm.mcpWorkflowRead' },
  { id: 'pm-workflow-write', labelKey: 'pages.projectDetail.pm.mcpWorkflowWrite' },
  { id: 'pm-agent-fs', labelKey: 'pages.projectDetail.pm.mcpAgentFs' },
] as const

type Tab = 'list' | 'edit' | 'notify'

const tab = ref<Tab>('list')
const loading = ref(true)
const saving = ref(false)
const channelList = ref<ChannelConfig[]>([])
const freeAgents = ref<string[]>([])
const secretsKeyConfigured = ref<boolean | undefined>(undefined)

const editingId = ref<string | null>(null)
const isNew = ref(false)
const chType = ref<'qq' | 'wecom'>('qq')
const saveError = ref('')
const notifyReceipts = ref<{ runId: string; kind: string; status?: string; error?: string; createdAt: string }[]>([])
const chEnabled = ref(true)
const chName = ref('')
const chAgent = ref('')
const chAppId = ref('')
const chAppSecret = ref('')
const chAppSecretSet = ref(false)
const chTurnTimeout = ref(0)
const chSandbox = ref(false)
const chAllowMemoryWrite = ref(false)
const chAllowSchedulerWrite = ref(false)
const chCronDeliver = ref(false)
const chCronDeliverTarget = ref('')
const chEnabledMcps = ref<string[]>([
  'pm-progress',
  'pm-workflow-read',
  'pm-workflow-write',
  'pm-agent-fs',
])
const chIsPrimary = ref(false)

const notifySelected = ref<string[]>([])
const notifySaving = ref(false)

const deleteOpen = ref(false)
const deleteTarget = ref<ChannelConfig | null>(null)
const deleteMode = ref<'promote' | 'none'>('promote')
const deleteNewPrimaryId = ref('')
const deleteSyncPmLeader = ref(true)

const TARGET_LISTBOX_ID = 'ch-multi-cron-target-listbox'
const recentTargets = ref<PushTargetOption[]>([])
const recentTargetsLoaded = ref(false)
const recentTargetsLoading = ref(false)
const targetComboOpen = ref(false)
const targetComboRoot = ref<HTMLElement | null>(null)
const targetActiveIndex = ref(-1)

const hasPrimary = computed(() => channelList.value.some((c) => c.isPrimary))
const addButtonLabel = computed(() =>
  hasPrimary.value
    ? t('pages.projectDetail.pm.channel.addSecondary')
    : t('pages.projectDetail.pm.channel.addPrimary'),
)

const agentOptions = computed(() => {
  const opts = new Set(freeAgents.value)
  if (chAgent.value) opts.add(chAgent.value)
  return [...opts].sort()
})

const editingChannel = computed(() =>
  editingId.value ? channelList.value.find((c) => c.id === editingId.value) || null : null,
)

function clearRecentTargetsCache() {
  recentTargets.value = []
  recentTargetsLoaded.value = false
  recentTargetsLoading.value = false
  targetComboOpen.value = false
  targetActiveIndex.value = -1
}

async function ensureRecentTargets() {
  if (!chCronDeliver.value) return
  if (recentTargetsLoaded.value || recentTargetsLoading.value) return
  recentTargetsLoading.value = true
  try {
    const res = await api.listPmThreads(props.projectId)
    recentTargets.value = deriveRecentPushTargets(res.items || [])
    recentTargetsLoaded.value = true
  } catch {
    recentTargets.value = []
    recentTargetsLoaded.value = true
  } finally {
    recentTargetsLoading.value = false
  }
}

function setTargetComboOpen(open: boolean) {
  targetComboOpen.value = open
  if (!open) targetActiveIndex.value = -1
  if (open) void ensureRecentTargets()
}

function selectPushTarget(value: string) {
  chCronDeliverTarget.value = value
  setTargetComboOpen(false)
}

function onTargetComboDocClick(ev: MouseEvent) {
  if (!targetComboOpen.value) return
  const el = targetComboRoot.value
  if (el && !el.contains(ev.target as Node)) setTargetComboOpen(false)
}

function onTargetInputKeydown(e: KeyboardEvent) {
  const opts = recentTargets.value
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (!targetComboOpen.value) setTargetComboOpen(true)
    if (!opts.length) return
    targetActiveIndex.value =
      targetActiveIndex.value < 0 ? 0 : (targetActiveIndex.value + 1) % opts.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (!targetComboOpen.value) setTargetComboOpen(true)
    if (!opts.length) return
    targetActiveIndex.value =
      targetActiveIndex.value < 0
        ? opts.length - 1
        : (targetActiveIndex.value - 1 + opts.length) % opts.length
  } else if (e.key === 'Enter' && targetComboOpen.value && targetActiveIndex.value >= 0) {
    e.preventDefault()
    const opt = opts[targetActiveIndex.value]
    if (opt) selectPushTarget(opt.value)
  } else if (e.key === 'Escape') {
    setTargetComboOpen(false)
  }
}

function toggleChMcp(id: string) {
  const set = new Set(chEnabledMcps.value)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  chEnabledMcps.value = PM_MCP_OPTIONS.map((o) => o.id).filter((x) => set.has(x))
}

function resetForm() {
  chType.value = 'qq'
  saveError.value = ''
  chEnabled.value = true
  chName.value = ''
  chAgent.value = ''
  chAppId.value = ''
  chAppSecret.value = ''
  chAppSecretSet.value = false
  chTurnTimeout.value = 0
  chSandbox.value = false
  chAllowMemoryWrite.value = false
  chAllowSchedulerWrite.value = false
  chCronDeliver.value = false
  chCronDeliverTarget.value = ''
  chEnabledMcps.value = [
    'pm-progress',
    'pm-workflow-read',
    'pm-workflow-write',
    'pm-agent-fs',
  ]
  chIsPrimary.value = false
  clearRecentTargetsCache()
}

function applyForm(ch: ChannelConfig) {
  editingId.value = ch.id
  isNew.value = false
  chType.value = ch.type === 'wecom' ? 'wecom' : 'qq'
  saveError.value = ''
  chEnabled.value = ch.enabled
  chName.value = ch.name || ''
  chAgent.value = ch.agentName || ''
  chAppId.value = ch.appId || ''
  chAppSecret.value = ''
  chAppSecretSet.value = !!ch.appSecretSet
  chTurnTimeout.value = ch.turnTimeoutSeconds || 0
  const cfg = (ch.config || {}) as Record<string, unknown>
  chSandbox.value = !!cfg.sandbox
  chAllowMemoryWrite.value = !!cfg.allowMemoryWrite
  chAllowSchedulerWrite.value = !!cfg.allowSchedulerWrite
  chCronDeliver.value = !!ch.cronDeliver
  chCronDeliverTarget.value = ch.cronDeliverTarget || ''
  chEnabledMcps.value = Array.isArray(ch.enabledMcps)
    ? [...ch.enabledMcps]
    : ['pm-progress', 'pm-workflow-read', 'pm-workflow-write', 'pm-agent-fs']
  chIsPrimary.value = !!ch.isPrimary
  clearRecentTargetsCache()
  if (chCronDeliver.value) void ensureRecentTargets()
}

async function load() {
  loading.value = true
  try {
    const [listRes, proj] = await Promise.all([
      api.listProjectChannels(props.projectId),
      api.getProject(props.projectId).catch(() => props.project),
    ])
    channelList.value = listRes.items || []
    freeAgents.value = listRes.freeAgents || []
    secretsKeyConfigured.value = listRes.secretsKeyConfigured
    const ids = Array.isArray(proj.notifyPolicy?.channelIds)
      ? [...(proj.notifyPolicy!.channelIds || [])]
      : Array.isArray(props.project.notifyPolicy?.channelIds)
        ? [...(props.project.notifyPolicy!.channelIds || [])]
        : []
    notifySelected.value = ids
    try {
      const rec = await api.listProjectNotifyReceipts(props.projectId)
      notifyReceipts.value = rec.items || []
    } catch {
      notifyReceipts.value = []
    }
  } catch (e: any) {
    toast.error(String(e?.message || e) || t('pages.projectDetail.pm.channel.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openAdd() {
  if (!freeAgents.value.length) {
    toast.error(t('pages.projectDetail.pm.channel.noFreeAgent'))
    return
  }
  resetForm()
  isNew.value = true
  editingId.value = null
  chIsPrimary.value = !hasPrimary.value
  const prefer = props.pmLeaderAgent || ''
  if (chIsPrimary.value && prefer && freeAgents.value.includes(prefer)) {
    chAgent.value = prefer
  } else {
    chAgent.value = freeAgents.value[0] || ''
  }
  tab.value = 'edit'
}

function openEdit(ch: ChannelConfig) {
  applyForm(ch)
  tab.value = 'edit'
}

function cancelEdit() {
  resetForm()
  isNew.value = false
  editingId.value = null
  tab.value = 'list'
}

function buildInput(): ChannelConfigInput {
  return {
    type: chType.value,
    name: chName.value.trim(),
    enabled: chEnabled.value,
    agentName: chAgent.value.trim(),
    enabledMcps: [...chEnabledMcps.value],
    appId: chAppId.value.trim(),
    appSecret: chAppSecret.value,
    turnTimeoutSeconds: Number(chTurnTimeout.value) || 0,
    cronDeliver: chCronDeliver.value,
    cronDeliverTarget: chCronDeliverTarget.value.trim(),
    config: {
      sandbox: chSandbox.value,
      allowMemoryWrite: chAllowMemoryWrite.value,
      allowSchedulerWrite: chAllowSchedulerWrite.value,
    },
  }
}

async function saveChannel() {
  saveError.value = ''
  if (!chAgent.value.trim()) {
    toast.error(t('pages.projectDetail.pm.channel.needAgent'))
    return
  }
  if (!chAppId.value.trim()) {
    toast.error(
      chType.value === 'wecom'
        ? t('pages.projectDetail.pm.channel.needBotId')
        : t('pages.projectDetail.pm.channel.needAppId'),
    )
    return
  }
  if (isNew.value && chType.value === 'wecom' && !chAppSecret.value.trim()) {
    toast.error(t('pages.projectDetail.pm.channel.needSecret'))
    return
  }
  const input = buildInput()
  // Primary rebind → confirm PmLeader sync when agent changes.
  if (!isNew.value && chIsPrimary.value) {
    const prev = editingChannel.value?.agentName || ''
    if (prev && input.agentName && prev !== input.agentName && input.agentName !== props.pmLeaderAgent) {
      const ok = window.confirm(t('pages.projectDetail.pm.channel.confirmSyncPmLeader'))
      if (ok) input.syncPmLeader = true
    }
  }
  saving.value = true
  try {
    if (isNew.value) {
      await api.createProjectChannel(props.projectId, input)
    } else if (editingId.value) {
      await api.updateProjectChannel(props.projectId, editingId.value, input)
    }
    toast.success(t('pages.projectDetail.saved'))
    await load()
    tab.value = 'list'
    resetForm()
    isNew.value = false
    editingId.value = null
  } catch (e: any) {
    const msg = String(e?.message || e) || t('pages.projectDetail.pm.channel.saveFailed')
    saveError.value = msg
    toast.error(msg)
  } finally {
    saving.value = false
  }
}

function askDelete(ch: ChannelConfig) {
  deleteTarget.value = ch
  if (ch.isPrimary) {
    deleteOpen.value = true
    deleteMode.value = channelList.value.filter((c) => c.id !== ch.id).length ? 'promote' : 'none'
    deleteNewPrimaryId.value =
      channelList.value.find((c) => c.id !== ch.id)?.id || ''
    deleteSyncPmLeader.value = true
  } else {
    if (!window.confirm(t('pages.projectDetail.pm.channel.deleteConfirm'))) return
    void doDelete(ch, { confirmNoPrimary: false })
  }
}

async function confirmDeletePrimary() {
  const ch = deleteTarget.value
  if (!ch) return
  if (deleteMode.value === 'promote') {
    if (!deleteNewPrimaryId.value) {
      toast.error(t('pages.projectDetail.pm.channel.needNewPrimary'))
      return
    }
    await doDelete(ch, {
      newPrimaryId: deleteNewPrimaryId.value,
      syncPmLeader: deleteSyncPmLeader.value,
    })
  } else {
    await doDelete(ch, { confirmNoPrimary: true })
  }
  deleteOpen.value = false
  deleteTarget.value = null
}

async function doDelete(
  ch: ChannelConfig,
  opts: { newPrimaryId?: string; confirmNoPrimary?: boolean; syncPmLeader?: boolean },
) {
  saving.value = true
  try {
    await api.deleteProjectChannelById(props.projectId, ch.id, opts)
    toast.success(t('pages.projectDetail.saved'))
    if (editingId.value === ch.id) cancelEdit()
    await load()
  } catch (e: any) {
    toast.error(String(e?.message || e) || t('pages.projectDetail.pm.channel.deleteFailed'))
  } finally {
    saving.value = false
  }
}

function toggleNotify(id: string) {
  const set = new Set(notifySelected.value)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  notifySelected.value = channelList.value.map((c) => c.id).filter((x) => set.has(x))
}

async function saveNotifyTargets() {
  notifySaving.value = true
  try {
    const notifyPolicy: ProjectNotifyPolicy = {
      ...(props.project.notifyPolicy || {}),
      enabled: props.project.notifyPolicy?.enabled !== false,
      defaultEvents: props.project.notifyPolicy?.defaultEvents || ['waiting_human', 'failed'],
      channelIds: [...notifySelected.value],
    }
    const updated = await api.updateProject(props.projectId, { notifyPolicy })
    emit('project-updated', updated)
    notifySelected.value = Array.isArray(updated.notifyPolicy?.channelIds)
      ? [...updated.notifyPolicy!.channelIds!]
      : []
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    notifySaving.value = false
  }
}

watch(
  () => props.projectId,
  () => {
    clearRecentTargetsCache()
    void load()
  },
)

watch(chCronDeliver, (on) => {
  if (!on) {
    clearRecentTargetsCache()
    return
  }
  void ensureRecentTargets()
})

onMounted(() => {
  document.addEventListener('mousedown', onTargetComboDocClick)
  void load()
})

onUnmounted(() => {
  document.removeEventListener('mousedown', onTargetComboDocClick)
})
</script>

<template>
  <div class="border-t border-line pt-4" data-testid="pm-channel-multi">
    <div class="mb-3 flex items-start justify-between gap-3">
      <div class="min-w-0">
        <strong class="block text-sm font-medium text-txt">
          {{ t('pages.projectDetail.pm.channel.sectionTitle') }}
        </strong>
        <p class="m-0 mt-1 text-xs leading-snug text-txt3">
          {{ t('pages.projectDetail.pm.channel.sectionHint') }}
        </p>
      </div>
    </div>

    <div class="mb-3 flex gap-0 border-b border-line" role="tablist">
      <button
        type="button"
        class="border-b-2 px-3.5 py-2.5 text-[13px]"
        :class="
          tab === 'list'
            ? 'border-accent text-txt'
            : 'border-transparent text-txt2 hover:text-txt'
        "
        data-testid="channel-tab-list"
        @click="tab = 'list'"
      >
        {{ t('pages.projectDetail.pm.channel.tabList') }}
      </button>
      <button
        type="button"
        class="border-b-2 px-3.5 py-2.5 text-[13px]"
        :class="
          tab === 'edit'
            ? 'border-accent text-txt'
            : 'border-transparent text-txt2 hover:text-txt'
        "
        data-testid="channel-tab-edit"
        @click="tab = 'edit'"
      >
        {{ t('pages.projectDetail.pm.channel.tabEdit') }}
      </button>
      <button
        type="button"
        class="border-b-2 px-3.5 py-2.5 text-[13px]"
        :class="
          tab === 'notify'
            ? 'border-accent text-txt'
            : 'border-transparent text-txt2 hover:text-txt'
        "
        data-testid="channel-tab-notify"
        @click="tab = 'notify'"
      >
        {{ t('pages.projectDetail.pm.channel.tabNotify') }}
      </button>
    </div>

    <div v-if="loading" class="text-sm text-txt3">{{ t('common.buttons.loading') }}</div>

    <!-- List -->
    <div v-else-if="tab === 'list'" data-testid="channel-panel-list">
      <div class="mb-3 flex items-center justify-between gap-2">
        <p class="m-0 text-xs text-txt2">{{ t('pages.projectDetail.pm.channel.listHint') }}</p>
        <AppButton variant="primary" data-testid="channel-add" @click="openAdd">
          {{ addButtonLabel }}
        </AppButton>
      </div>

      <div v-if="!channelList.length" class="border border-dashed border-line-strong p-3.5 text-xs text-txt3">
        {{ t('pages.projectDetail.pm.channel.listEmpty') }}
      </div>
      <div v-else class="border border-line">
        <div
          class="grid grid-cols-[1fr_100px_140px_80px_120px] gap-3 border-b border-line bg-elevated px-3.5 py-2 text-[11px] uppercase tracking-wide text-txt3 max-md:grid-cols-2"
        >
          <span>{{ t('pages.projectDetail.pm.channel.colName') }}</span>
          <span>{{ t('pages.projectDetail.pm.channel.colRole') }}</span>
          <span class="max-md:hidden">{{ t('pages.projectDetail.pm.channel.colAgent') }}</span>
          <span class="max-md:hidden">{{ t('pages.projectDetail.pm.channel.colStatus') }}</span>
          <span>{{ t('pages.projectDetail.pm.channel.colActions') }}</span>
        </div>
        <div
          v-for="ch in channelList"
          :key="ch.id"
          class="grid grid-cols-[1fr_100px_140px_80px_120px] items-center gap-3 border-b border-line px-3.5 py-3 last:border-b-0 hover:bg-elevated max-md:grid-cols-2"
          data-testid="channel-row"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2 text-[13px] font-medium text-txt">
              <span
                class="border px-2 py-0.5 text-[11px]"
                data-testid="channel-type-badge"
                :class="
                  ch.type === 'wecom'
                    ? 'border-accent/55 bg-accent-dim text-accent-2'
                    : 'border-info/45 text-info'
                "
              >
                {{
                  ch.type === 'wecom'
                    ? t('pages.projectDetail.pm.channel.typeWecom')
                    : t('pages.projectDetail.pm.channel.typeQq')
                }}
              </span>
              <span class="truncate">{{ ch.name || ch.appId }}</span>
              <span
                v-if="ch.isPrimary"
                class="border border-accent/55 bg-accent-dim px-2 py-0.5 text-[11px] text-accent-2"
              >
                {{ t('pages.projectDetail.pm.channel.rolePrimary') }}
              </span>
            </div>
            <div class="mt-0.5 font-mono text-[11px] text-txt3">{{ ch.appId }}</div>
          </div>
          <div class="text-[12px] text-txt2">
            {{
              ch.isPrimary
                ? t('pages.projectDetail.pm.channel.rolePrimary')
                : t('pages.projectDetail.pm.channel.roleSecondary')
            }}
          </div>
          <div class="truncate font-mono text-[12px] text-txt2 max-md:hidden">
            {{ ch.agentName || '—' }}
          </div>
          <div class="flex flex-col gap-1 text-[12px] max-md:hidden">
            <span :class="ch.enabled ? 'text-ok' : 'text-txt3'">
              {{
                ch.enabled
                  ? t('pages.projectDetail.pm.channel.statusOn')
                  : t('pages.projectDetail.pm.channel.statusOff')
              }}
            </span>
            <span class="inline-flex items-center gap-1.5" data-testid="channel-online">
              <span
                class="inline-block h-1.5 w-1.5"
                :class="ch.online ? 'bg-ok' : 'bg-txt3'"
              />
              {{
                ch.online
                  ? t('pages.projectDetail.pm.channel.online')
                  : t('pages.projectDetail.pm.channel.offline')
              }}
            </span>
          </div>
          <div class="flex flex-wrap gap-1.5">
            <AppButton variant="ghost" class="!px-2 !py-1 text-[12px]" @click="openEdit(ch)">
              {{ t('pages.projectDetail.pm.channel.edit') }}
            </AppButton>
            <AppButton
              variant="ghost"
              class="!px-2 !py-1 text-[12px] text-err"
              :disabled="saving"
              @click="askDelete(ch)"
            >
              {{ t('pages.projectDetail.pm.channel.delete') }}
            </AppButton>
          </div>
        </div>
      </div>

      <div class="mt-3 border border-info/35 bg-info/10 px-3 py-2.5 text-xs leading-snug text-txt2">
        <strong class="text-info">{{ t('pages.projectDetail.pm.channel.compatTitle') }}</strong>
        {{ t('pages.projectDetail.pm.channel.compatHint') }}
      </div>
    </div>

    <!-- Edit -->
    <div v-else-if="tab === 'edit'" data-testid="channel-panel-edit">
      <div class="mb-3 flex items-center justify-between gap-2">
        <div>
          <strong class="block text-sm text-txt">
            {{
              isNew
                ? addButtonLabel
                : t('pages.projectDetail.pm.channel.editTitle')
            }}
          </strong>
          <p class="m-0 mt-1 text-xs text-txt2">
            {{
              chIsPrimary || (isNew && !hasPrimary)
                ? t('pages.projectDetail.pm.channel.editPrimaryHint')
                : t('pages.projectDetail.pm.channel.editSecondaryHint')
            }}
          </p>
        </div>
        <AppButton variant="ghost" @click="cancelEdit">
          {{ t('pages.projectDetail.pm.channel.backToList') }}
        </AppButton>
      </div>

      <div v-if="!isNew && !editingId" class="border border-dashed border-line-strong p-3.5 text-xs text-txt3">
        {{ t('pages.projectDetail.pm.channel.editEmpty') }}
      </div>

      <template v-else>
        <div class="grid gap-4 lg:grid-cols-[1.15fr_.85fr]">
          <div class="border border-line bg-surface p-4">
            <h3 class="m-0 text-[13px] font-semibold text-txt">
              {{ t('pages.projectDetail.pm.channel.basicsTitle') }}
            </h3>
            <p class="m-0 mb-3.5 mt-1 text-xs text-txt2">
              {{ t('pages.projectDetail.pm.channel.basicsHint') }}
            </p>

            <div class="mb-3">
              <span class="label">{{ t('pages.projectDetail.pm.channel.typeLabel') }}</span>
              <div class="mt-1 flex border border-line" data-testid="channel-type-switch">
                <button
                  type="button"
                  class="flex-1 px-3 py-2 text-[13px]"
                  :class="chType === 'qq' ? 'bg-accent-dim text-txt' : 'bg-base text-txt2'"
                  :disabled="!isNew"
                  data-testid="channel-type-qq"
                  @click="isNew && (chType = 'qq')"
                >
                  {{ t('pages.projectDetail.pm.channel.typeQq') }}
                </button>
                <button
                  type="button"
                  class="flex-1 border-l border-line px-3 py-2 text-[13px]"
                  :class="chType === 'wecom' ? 'bg-accent-dim text-txt' : 'bg-base text-txt2'"
                  :disabled="!isNew"
                  data-testid="channel-type-wecom"
                  @click="isNew && (chType = 'wecom')"
                >
                  {{ t('pages.projectDetail.pm.channel.typeWecom') }}
                </button>
              </div>
              <p v-if="!isNew" class="mt-1 text-[11px] text-txt3">
                {{ t('pages.projectDetail.pm.channel.typeFrozen') }}
              </p>
            </div>

            <label class="flex items-center justify-between gap-3 text-[13px] text-txt">
              <span>{{ t('pages.projectDetail.pm.channel.enable') }}</span>
              <AppSwitch v-model="chEnabled" :aria-label="t('pages.projectDetail.pm.channel.enable')" />
            </label>

            <div class="mt-3">
              <label class="label" for="ch-multi-name">{{ t('pages.projectDetail.pm.channel.name') }}</label>
              <input
                id="ch-multi-name"
                v-model="chName"
                class="input max-w-md"
                :placeholder="t('pages.projectDetail.pm.channel.namePlaceholder')"
              />
            </div>

            <div class="mt-3">
              <label class="label" for="ch-multi-agent">
                {{ t('pages.projectDetail.pm.channel.bindAgent') }}
              </label>
              <select id="ch-multi-agent" v-model="chAgent" class="input max-w-md">
                <option value="">{{ t('pages.projectDetail.pm.selectAgent') }}</option>
                <option v-for="name in agentOptions" :key="name" :value="name">{{ name }}</option>
              </select>
              <p class="mt-1 text-[11px] text-txt3">
                {{ t('pages.projectDetail.pm.channel.bindAgentHint') }}
              </p>
            </div>

            <div class="mt-3 grid grid-cols-2 gap-3 max-sm:grid-cols-1">
              <div>
                <label class="label" for="ch-multi-appid">
                  {{
                    chType === 'wecom'
                      ? t('pages.projectDetail.pm.channel.botId')
                      : t('pages.projectDetail.pm.channel.appId')
                  }}
                </label>
                <input
                  id="ch-multi-appid"
                  v-model="chAppId"
                  class="input w-full"
                  :disabled="!isNew && chType === 'wecom'"
                  data-testid="channel-appid"
                />
              </div>
              <div>
                <label class="label" for="ch-multi-secret">
                  {{
                    chType === 'wecom'
                      ? t('pages.projectDetail.pm.channel.secret')
                      : t('pages.projectDetail.pm.channel.appSecret')
                  }}
                </label>
                <input
                  id="ch-multi-secret"
                  v-model="chAppSecret"
                  type="password"
                  class="input w-full"
                  :placeholder="chAppSecretSet ? t('pages.projectDetail.pm.channel.appSecretKeep') : ''"
                  data-testid="channel-secret"
                />
                <p v-if="chAppSecretSet" class="mt-1 text-[11px] text-txt3">
                  {{ t('pages.projectDetail.pm.channel.appSecretSet') }}
                </p>
              </div>
            </div>
            <p
              v-if="saveError"
              class="mt-2 border border-err/45 bg-err/10 px-3 py-2 text-xs text-err"
              data-testid="channel-save-error"
            >
              {{ saveError }}
            </p>

            <div class="mt-3 grid grid-cols-2 gap-3 max-sm:grid-cols-1">
              <div>
                <label class="label" for="ch-multi-timeout">
                  {{ t('pages.projectDetail.pm.channel.turnTimeout') }}
                </label>
                <input
                  id="ch-multi-timeout"
                  v-model.number="chTurnTimeout"
                  type="number"
                  min="0"
                  class="input w-[140px]"
                />
                <p class="mt-1 text-[11px] text-txt3">
                  {{ t('pages.projectDetail.pm.channel.turnTimeoutHint') }}
                </p>
              </div>
              <label
                v-if="chType === 'qq'"
                class="flex items-center justify-between gap-3 pt-6 text-[13px] text-txt"
                data-testid="channel-sandbox"
              >
                <span>{{ t('pages.projectDetail.pm.channel.sandbox') }}</span>
                <AppSwitch v-model="chSandbox" :aria-label="t('pages.projectDetail.pm.channel.sandbox')" />
              </label>
            </div>

            <div class="mt-3 border border-line p-3">
              <label class="flex cursor-pointer items-center gap-2.5 text-[13px] text-txt">
                <AppSwitch
                  v-model="chCronDeliver"
                  data-testid="cron-deliver-enable"
                  :aria-label="t('pages.projectDetail.pm.channel.cronDeliver')"
                />
                <span>{{ t('pages.projectDetail.pm.channel.cronDeliver') }}</span>
              </label>
              <p class="mt-1.5 text-[11px] leading-snug text-txt3">
                {{ t('pages.projectDetail.pm.channel.cronDeliverHintMulti') }}
              </p>
              <div v-if="chCronDeliver" class="mt-3">
                <label class="label" for="ch-multi-cron">
                  {{ t('pages.projectDetail.pm.channel.cronDeliverTarget') }}
                </label>
                <div ref="targetComboRoot" class="relative">
                  <div class="flex gap-1">
                    <input
                      id="ch-multi-cron"
                      v-model="chCronDeliverTarget"
                      class="input w-full flex-1 font-mono text-[12px]"
                      placeholder="guild:123"
                      autocomplete="off"
                      role="combobox"
                      data-testid="cron-deliver-target-input"
                      @focus="setTargetComboOpen(true)"
                      @keydown="onTargetInputKeydown"
                    />
                    <button
                      type="button"
                      class="chip shrink-0 px-2 hover:border-accent/50"
                      @click="setTargetComboOpen(!targetComboOpen)"
                    >
                      <Icon name="chevron-down" :size="14" />
                    </button>
                  </div>
                  <div
                    v-if="targetComboOpen"
                    :id="TARGET_LISTBOX_ID"
                    class="card scroll-area absolute left-0 right-0 z-20 mt-1 max-h-64 overflow-y-auto"
                    role="listbox"
                  >
                    <template v-if="recentTargets.length">
                      <button
                        v-for="(opt, idx) in recentTargets"
                        :key="opt.value"
                        type="button"
                        class="block w-full px-3 py-2 text-left hover:bg-base"
                        :class="idx === targetActiveIndex ? 'bg-base' : ''"
                        @click="selectPushTarget(opt.value)"
                      >
                        <span class="block truncate text-[12px] text-txt">
                          {{ pushTargetPrimaryLabel(opt) }}
                        </span>
                        <span class="mt-0.5 block font-mono text-[11px] text-txt3">
                          {{ opt.value }}
                          <template v-if="opt.unspoken"> · {{ t('pages.projectDetail.pm.unspoken') }}</template>
                        </span>
                      </button>
                    </template>
                    <p v-else class="px-3 py-2 text-[11px] text-txt3">
                      {{ t('pages.projectDetail.pm.channel.recentTargetsEmpty') }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="grid gap-4 content-start">
            <div class="border border-line bg-surface p-4">
              <h3 class="m-0 text-[13px] font-semibold text-txt">
                {{ t('pages.projectDetail.pm.channel.sessionCapsTitle') }}
              </h3>
              <p class="m-0 mb-3 mt-1 text-xs text-txt2">
                {{ t('pages.projectDetail.pm.channel.sessionCapsHint') }}
              </p>
              <label class="flex cursor-pointer items-start gap-2.5 border-b border-line py-2.5 text-[13px]">
                <AppSwitch v-model="chAllowMemoryWrite" class="mt-0.5" />
                <span>
                  <span class="block">{{ t('pages.projectDetail.pm.channel.allowMemoryWrite') }}</span>
                  <span class="mt-0.5 block text-[11px] text-txt3">
                    {{ t('pages.projectDetail.pm.channel.allowMemoryWriteHint') }}
                  </span>
                </span>
              </label>
              <label class="flex cursor-pointer items-start gap-2.5 py-2.5 text-[13px]">
                <AppSwitch v-model="chAllowSchedulerWrite" class="mt-0.5" />
                <span>
                  <span class="block">{{ t('pages.projectDetail.pm.channel.allowSchedulerWrite') }}</span>
                  <span class="mt-0.5 block text-[11px] text-txt3">
                    {{ t('pages.projectDetail.pm.channel.allowSchedulerWriteHint') }}
                  </span>
                </span>
              </label>
            </div>

            <div class="border border-line bg-surface p-4">
              <h3 class="m-0 text-[13px] font-semibold text-txt">
                {{ t('pages.projectDetail.pm.channel.channelMcpsTitle') }}
              </h3>
              <p class="m-0 mb-2 mt-1 text-xs text-txt2">
                {{ t('pages.projectDetail.pm.channel.channelMcpsHint') }}
              </p>
              <div class="mb-3 border border-info/35 bg-info/10 px-3 py-2 text-xs text-txt2">
                {{ t('pages.projectDetail.pm.channel.channelMcpsNote') }}
              </div>
              <div class="grid gap-2">
                <label
                  v-for="opt in PM_MCP_OPTIONS"
                  :key="opt.id"
                  class="flex cursor-pointer items-center justify-between gap-2 border border-line bg-base px-3 py-2.5 text-[13px]"
                >
                  <span class="flex items-center gap-2">
                    <AppSwitch
                      :model-value="chEnabledMcps.includes(opt.id)"
                      @update:model-value="toggleChMcp(opt.id)"
                    />
                    <code class="font-mono text-[12px] text-accent-2">{{ opt.id }}</code>
                  </span>
                  <span class="text-[11px] text-txt3">{{ t(opt.labelKey) }}</span>
                </label>
              </div>
            </div>
          </div>
        </div>

        <p
          v-if="secretsKeyConfigured === false"
          class="mt-3 text-[11px] text-err"
        >
          {{ t('pages.projectDetail.pm.channel.secretKeyHint') }}
        </p>

        <div class="mt-3 flex flex-wrap gap-2">
          <AppButton variant="primary" :disabled="saving" data-testid="channel-save" @click="saveChannel">
            {{ saving ? t('common.buttons.saving') : t('pages.projectDetail.pm.channel.saveButton') }}
          </AppButton>
          <AppButton variant="ghost" :disabled="saving" @click="cancelEdit">
            {{ t('common.buttons.cancel') }}
          </AppButton>
          <AppButton
            v-if="editingId && !isNew"
            variant="ghost"
            class="text-err"
            :disabled="saving"
            @click="editingChannel && askDelete(editingChannel)"
          >
            {{ t('pages.projectDetail.pm.channel.delete') }}
          </AppButton>
        </div>
      </template>
    </div>

    <!-- Notify targets -->
    <div v-else data-testid="channel-panel-notify">
      <div class="mb-3 flex items-center justify-between gap-2">
        <div>
          <strong class="block text-sm text-txt">
            {{ t('pages.projectDetail.pm.channel.notifyTitle') }}
          </strong>
          <p class="m-0 mt-1 text-xs text-txt2">
            {{ t('pages.projectDetail.pm.channel.notifyHint') }}
          </p>
        </div>
        <AppButton
          variant="primary"
          :disabled="notifySaving"
          data-testid="channel-notify-save"
          @click="saveNotifyTargets"
        >
          {{
            notifySaving
              ? t('common.buttons.saving')
              : t('pages.projectDetail.pm.channel.notifySave')
          }}
        </AppButton>
      </div>

      <div class="mb-3 flex items-center justify-between gap-2 border border-line bg-surface px-3.5 py-3">
        <div>
          <div class="text-[13px] font-medium text-txt">
            {{ t('pages.projectDetail.pm.channel.notifyPolicyTitle') }}
          </div>
          <div class="mt-0.5 text-[11px] text-txt3">
            {{ t('pages.projectDetail.pm.channel.notifyPolicyHint') }}
          </div>
        </div>
        <span class="border border-line bg-elevated px-2 py-0.5 text-[12px] text-txt2">
          {{ t('pages.projectDetail.pm.channel.notifySelected', { n: notifySelected.length }) }}
        </span>
      </div>

      <div v-if="!channelList.length" class="border border-dashed border-line-strong p-3.5 text-xs text-txt3">
        {{ t('pages.projectDetail.pm.channel.listEmpty') }}
      </div>
      <div v-else class="border border-line">
        <label
          v-for="ch in channelList"
          :key="ch.id"
          class="flex cursor-pointer items-center gap-3 border-b border-line px-3.5 py-3 last:border-b-0"
        >
          <input
            type="checkbox"
            class="accent-[rgb(var(--c-accent))]"
            :checked="notifySelected.includes(ch.id)"
            @change="toggleNotify(ch.id)"
          />
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2 text-[13px] text-txt">
              <span>{{ ch.name || ch.appId }}</span>
              <span
                v-if="ch.isPrimary"
                class="border border-accent/55 bg-accent-dim px-2 py-0.5 text-[11px] text-accent-2"
              >
                {{ t('pages.projectDetail.pm.channel.rolePrimary') }}
              </span>
              <span
                v-if="!ch.enabled"
                class="border border-line px-2 py-0.5 text-[11px] text-txt3"
              >
                {{ t('pages.projectDetail.pm.channel.statusOff') }}
              </span>
            </div>
            <div class="mt-0.5 font-mono text-[11px] text-txt3">
              {{ ch.agentName || '—' }} · {{ ch.cronDeliverTarget || t('pages.projectDetail.pm.channel.noPushTarget') }}
            </div>
          </div>
        </label>
      </div>

      <div
        v-if="!notifySelected.length"
        class="mt-2.5 border border-dashed border-line-strong bg-base p-3.5 text-xs text-txt3"
        data-testid="notify-empty-hint"
      >
        {{ t('pages.projectDetail.pm.channel.notifyEmpty') }}
      </div>

      <div class="mt-4 border border-line" data-testid="channel-deliver-log">
        <div class="border-b border-line bg-elevated px-3.5 py-2 text-[13px] font-medium text-txt">
          {{ t('pages.projectDetail.pm.channel.deliverLogTitle') }}
        </div>
        <div v-if="!notifyReceipts.length" class="px-3.5 py-3 text-xs text-txt3">
          {{ t('pages.projectDetail.pm.channel.deliverLogEmpty') }}
        </div>
        <div
          v-for="(rec, idx) in notifyReceipts"
          :key="idx"
          class="flex items-start gap-2 border-b border-line px-3.5 py-2.5 last:border-b-0"
        >
          <span
            class="shrink-0 border px-2 py-0.5 text-[11px]"
            :class="rec.status === 'ok' ? 'border-ok/40 text-ok' : 'border-err/45 text-err'"
          >
            {{ rec.status === 'ok' ? t('pages.projectDetail.pm.channel.deliverOk') : t('pages.projectDetail.pm.channel.deliverFail') }}
          </span>
          <div class="min-w-0">
            <div class="font-mono text-[12px] text-txt">{{ rec.kind }} · {{ rec.runId }}</div>
            <div v-if="rec.error" class="mt-0.5 text-[11px] text-err">{{ rec.error }}</div>
          </div>
        </div>
      </div>

      <div class="mt-3 border border-warn/40 bg-warn/10 px-3 py-2.5 text-xs leading-snug text-txt2">
        {{ t('pages.projectDetail.pm.channel.notifyFanoutHint') }}
      </div>
    </div>

    <!-- Delete primary modal -->
    <div
      v-if="deleteOpen && deleteTarget"
      class="fixed inset-0 z-40 flex items-center justify-center bg-black/55 p-6"
      data-testid="channel-delete-primary-modal"
    >
      <div class="w-full max-w-md border border-line-strong bg-surface p-4 shadow-[var(--shadow-card)]">
        <h3 class="m-0 text-sm font-semibold text-txt">
          {{ t('pages.projectDetail.pm.channel.deletePrimaryTitle') }}
        </h3>
        <p class="mb-3.5 mt-2 text-xs text-txt2">
          {{ t('pages.projectDetail.pm.channel.deletePrimaryHint') }}
        </p>
        <div class="mb-3.5 grid gap-2">
          <button
            type="button"
            class="border px-3 py-2.5 text-left text-xs"
            :class="
              deleteMode === 'promote'
                ? 'border-accent bg-accent-dim text-txt'
                : 'border-line bg-base text-txt2'
            "
            @click="deleteMode = 'promote'"
          >
            {{ t('pages.projectDetail.pm.channel.deletePrimaryPromote') }}
          </button>
          <button
            type="button"
            class="border px-3 py-2.5 text-left text-xs"
            :class="
              deleteMode === 'none'
                ? 'border-accent bg-accent-dim text-txt'
                : 'border-line bg-base text-txt2'
            "
            @click="deleteMode = 'none'"
          >
            {{ t('pages.projectDetail.pm.channel.deletePrimaryNone') }}
          </button>
        </div>
        <div v-if="deleteMode === 'promote'" class="mb-3">
          <label class="label">{{ t('pages.projectDetail.pm.channel.newPrimary') }}</label>
          <select v-model="deleteNewPrimaryId" class="input">
            <option
              v-for="ch in channelList.filter((c) => c.id !== deleteTarget?.id)"
              :key="ch.id"
              :value="ch.id"
            >
              {{ ch.name || ch.appId }} ({{ ch.agentName || '—' }})
            </option>
          </select>
          <label class="mt-2 flex items-center gap-2 text-[12px] text-txt2">
            <input v-model="deleteSyncPmLeader" type="checkbox" class="accent-[rgb(var(--c-accent))]" />
            {{ t('pages.projectDetail.pm.channel.syncPmLeaderOnPromote') }}
          </label>
        </div>
        <div class="flex justify-end gap-2">
          <AppButton variant="ghost" @click="deleteOpen = false">
            {{ t('common.buttons.cancel') }}
          </AppButton>
          <AppButton variant="primary" :disabled="saving" @click="confirmDeletePrimary">
            {{ t('pages.projectDetail.pm.channel.confirmDelete') }}
          </AppButton>
        </div>
      </div>
    </div>
  </div>
</template>

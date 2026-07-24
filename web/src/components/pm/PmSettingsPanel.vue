<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import Icon from '@/components/ui/Icon.vue'
import { api } from '@/lib/api'
import {
  deriveRecentPushTargets,
  pushTargetPrimaryLabel,
  type PushTargetOption,
} from '@/lib/cronDeliverTargets'
import { useToast } from '@/lib/useToast'
import type { Agent, ChannelConfig, ChannelConfigInput } from '@/lib/api'
import type { PmLeaderBinding } from '@/lib/types'

const props = defineProps<{ projectId: string }>()
const emit = defineEmits<{ changed: [PmLeaderBinding] }>()

const { t } = useI18n()
const toast = useToast()

const PM_MCP_OPTIONS = [
  { id: 'pm-progress', labelKey: 'pages.projectDetail.pm.mcpProgress' },
  { id: 'pm-workflow-read', labelKey: 'pages.projectDetail.pm.mcpWorkflowRead' },
  { id: 'pm-workflow-write', labelKey: 'pages.projectDetail.pm.mcpWorkflowWrite' },
] as const

const binding = ref<PmLeaderBinding | null>(null)
const agents = ref<Agent[]>([])
const selectedAgent = ref('')
const enabled = ref(false)
const enabledMcps = ref<string[]>(['pm-progress', 'pm-workflow-read', 'pm-workflow-write'])
const loading = ref(true)
const saving = ref(false)

// Per-project QQ channel (one per project). type is fixed to "qq" server-side.
const channel = ref<ChannelConfig | null>(null)
const chEnabled = ref(false)
const chName = ref('')
const chAppId = ref('')
const chAppSecret = ref('')
const chAppSecretSet = ref(false)
const chTurnTimeout = ref(0)
const chSandbox = ref(false)
const chAllowMemoryWrite = ref(false)
const chAllowSchedulerWrite = ref(false)
const chCronDeliver = ref(false)
const chCronDeliverTarget = ref('')
const chSaving = ref(false)
// false only when the server explicitly reports the encryption key is missing;
// undefined/true hides the scary "configure secrets_key" hint.
const secretsKeyConfigured = ref<boolean | undefined>(undefined)

// Cron deliver target Combobox: recent QQ channel sessions (max 10).
const TARGET_LISTBOX_ID = 'ch-cron-target-listbox'
const recentTargets = ref<PushTargetOption[]>([])
const recentTargetsLoaded = ref(false)
const recentTargetsLoading = ref(false)
const targetComboOpen = ref(false)
const targetComboRoot = ref<HTMLElement | null>(null)
const targetActiveIndex = ref(-1)

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
  } catch (e: any) {
    recentTargets.value = []
    recentTargetsLoaded.value = true
    const detail = String(e?.message || e || '').trim()
    const base = t('pages.projectDetail.pm.channel.recentTargetsLoadFailed')
    toast.error(detail && detail !== base ? `${base}（${detail}）` : base)
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
  if (el && !el.contains(ev.target as Node)) {
    setTargetComboOpen(false)
  }
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

const showChannelPmWarning = computed(
  () => chEnabled.value && !enabled.value,
)

const noAgents = computed(() => !loading.value && agents.value.length === 0)
const controlsDisabled = computed(() => noAgents.value || saving.value)

type BadgeKind = 'ok' | 'off' | 'err'

const statusBadge = computed(() => {
  // Align with noAgents: never treat an in-flight load as "unavailable".
  if (loading.value) return null
  if (agents.value.length === 0) {
    return {
      kind: 'err' as BadgeKind,
      label: t('pages.projectDetail.pm.badgeUnavailable'),
      agentName: '',
    }
  }
  if (binding.value?.agentError) {
    return {
      kind: 'err' as BadgeKind,
      label: t('pages.projectDetail.pm.badgeError'),
      agentName: selectedAgent.value,
    }
  }
  if (enabled.value && selectedAgent.value) {
    return {
      kind: 'ok' as BadgeKind,
      label: t('pages.projectDetail.pm.badgeEnabled'),
      agentName: selectedAgent.value,
    }
  }
  if (enabled.value && !selectedAgent.value) {
    return {
      kind: 'err' as BadgeKind,
      label: t('pages.projectDetail.pm.badgePending'),
      agentName: '',
    }
  }
  return {
    kind: 'off' as BadgeKind,
    label: t('pages.projectDetail.pm.badgeDisabled'),
    agentName: '',
  }
})

const badgeClass = computed(() => {
  if (!statusBadge.value) return ''
  if (statusBadge.value.kind === 'ok') return 'border-ok/35 bg-ok/10 text-ok'
  if (statusBadge.value.kind === 'err') return 'border-err/40 bg-err/12 text-err'
  return 'border-line bg-elevated text-txt2'
})

const badgeDotClass = computed(() => {
  if (!statusBadge.value) return ''
  if (statusBadge.value.kind === 'ok') return 'bg-ok'
  if (statusBadge.value.kind === 'err') return 'bg-err'
  return 'bg-txt3'
})

const agentOptions = computed((): { name: string; stale: boolean }[] => {
  const names = agents.value.map((a) => a.name)
  const current = selectedAgent.value
  const options = agents.value.map((a) => ({ name: a.name, stale: false }))
  if (current && !names.includes(current)) {
    options.push({ name: current, stale: true })
  }
  return options
})

const aclText = computed(
  () => binding.value?.aclNote || t('pages.projectDetail.pm.aclMemoryNote'),
)

function toggleEnabled() {
  if (controlsDisabled.value) return
  enabled.value = !enabled.value
}

function onEnableKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    toggleEnabled()
  }
}

function toggleMcp(id: string) {
  if (controlsDisabled.value) return
  const set = new Set(enabledMcps.value)
  if (set.has(id)) set.delete(id)
  else set.add(id)
  enabledMcps.value = PM_MCP_OPTIONS.map((o) => o.id).filter((x) => set.has(x))
}

function applyChannel(ch: ChannelConfig | null) {
  channel.value = ch
  chEnabled.value = ch?.enabled ?? false
  chName.value = ch?.name ?? ''
  chAppId.value = ch?.appId ?? ''
  chAppSecret.value = ''
  chAppSecretSet.value = ch?.appSecretSet ?? false
  chTurnTimeout.value = ch?.turnTimeoutSeconds ?? 0
  const cfg = (ch?.config || {}) as Record<string, unknown>
  chSandbox.value = !!cfg.sandbox
  chAllowMemoryWrite.value = !!cfg.allowMemoryWrite
  chAllowSchedulerWrite.value = !!cfg.allowSchedulerWrite
  chCronDeliver.value = ch?.cronDeliver ?? false
  // Orphan values stay in the input; list options never clear this field.
  chCronDeliverTarget.value = ch?.cronDeliverTarget ?? ''
}

async function refreshSecretsKeyStatus() {
  try {
    const ch = await api.getProjectChannel(props.projectId)
    secretsKeyConfigured.value = ch.secretsKeyConfigured
  } catch {
    // Keep the last known value if the status probe fails.
  }
}

async function load() {
  loading.value = true
  try {
    const [b, list, ch] = await Promise.all([
      api.getPmLeader(props.projectId),
      api.listAgents(),
      api.getProjectChannel(props.projectId).catch(() => ({
        channel: null as ChannelConfig | null,
        secretsKeyConfigured: undefined as boolean | undefined,
      })),
    ])
    binding.value = b
    agents.value = list || []
    selectedAgent.value = b.agentConfigRef || ''
    enabled.value = agents.value.length === 0 ? false : b.enabled
    enabledMcps.value = Array.isArray(b.enabledMcps)
      ? [...b.enabledMcps]
      : ['pm-progress', 'pm-workflow-read', 'pm-workflow-write']
    applyChannel(ch.channel)
    secretsKeyConfigured.value = ch.secretsKeyConfigured
    // Project switch may keep chCronDeliver===true (watch won't re-fire); fetch if needed.
    if (chCronDeliver.value) void ensureRecentTargets()
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    loading.value = false
  }
}

async function saveChannel() {
  chSaving.value = true
  try {
    const input: ChannelConfigInput = {
      name: chName.value.trim(),
      enabled: chEnabled.value,
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
    const saved = await api.putProjectChannel(props.projectId, input)
    applyChannel(saved)
    await refreshSecretsKeyStatus()
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    await refreshSecretsKeyStatus()
    toast.error(String(e?.message || e) || t('pages.projectDetail.pm.channel.saveFailed'))
  } finally {
    chSaving.value = false
  }
}

async function deleteChannel() {
  if (!channel.value) return
  if (!window.confirm(t('pages.projectDetail.pm.channel.deleteConfirm'))) return
  chSaving.value = true
  try {
    await api.deleteProjectChannel(props.projectId)
    applyChannel(null)
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e) || t('pages.projectDetail.pm.channel.deleteFailed'))
  } finally {
    chSaving.value = false
  }
}

async function save() {
  if (noAgents.value) {
    toast.error(t('pages.projectDetail.pm.noAgents'))
    return
  }
  if (enabled.value && !selectedAgent.value) {
    toast.error(t('pages.projectDetail.pm.needAgent'))
    return
  }
  saving.value = true
  try {
    binding.value = await api.updatePmLeader(props.projectId, {
      enabled: enabled.value,
      agentConfigRef: selectedAgent.value,
      enabledMcps: [...enabledMcps.value],
    })
    enabled.value = binding.value.enabled
    selectedAgent.value = binding.value.agentConfigRef || ''
    enabledMcps.value = Array.isArray(binding.value.enabledMcps)
      ? [...binding.value.enabledMcps]
      : ['pm-progress', 'pm-workflow-read', 'pm-workflow-write']
    emit('changed', binding.value)
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    saving.value = false
  }
}

watch(noAgents, (empty) => {
  if (empty) enabled.value = false
})

watch(selectedAgent, (name) => {
  if (!binding.value?.agentError) return
  if (name && agents.value.some((a) => a.name === name)) {
    binding.value = { ...binding.value, agentError: '' }
  }
})

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
  // First time the control is needed after checking: fetch & cache.
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
  <div
    class="flex min-h-0 flex-1 flex-col overflow-hidden border border-line bg-surface shadow-[var(--shadow-card)]"
  >
    <div
      class="flex shrink-0 flex-wrap items-start justify-between gap-3 border-b border-line bg-elevated/55 px-4 py-3.5"
    >
      <div class="min-w-0 max-w-[42em]">
        <h2 class="m-0 text-sm font-semibold text-txt">
          {{ t('pages.projectDetail.pm.settingsTitle') }}
        </h2>
        <p class="m-0 mt-1 text-[13px] leading-snug text-txt3">
          {{ t('pages.projectDetail.pm.settingsHint') }}
        </p>
      </div>
      <div
        v-if="statusBadge"
        class="inline-flex max-w-full items-center gap-1.5 whitespace-nowrap border px-2.5 py-1 text-xs"
        :class="badgeClass"
        role="status"
      >
        <span class="h-1.5 w-1.5 shrink-0" :class="badgeDotClass" aria-hidden="true" />
        <span>{{ statusBadge.label }}</span>
        <span
          v-if="statusBadge.agentName"
          class="max-w-[160px] truncate opacity-90"
          :title="statusBadge.agentName"
        >
          {{ statusBadge.agentName }}
        </span>
      </div>
    </div>

    <div class="scroll-area flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
      <div
        class="flex items-start gap-2.5 border border-accent-2/20 bg-accent-dim/85 px-3 py-2.5 text-[13px] leading-snug text-txt"
      >
        <svg
          class="mt-0.5 h-3.5 w-3.5 shrink-0 stroke-accent-2"
          viewBox="0 0 24 24"
          fill="none"
          stroke-width="1.75"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="9" />
          <path d="M12 8v5M12 16h.01" stroke-linecap="square" />
        </svg>
        <span>{{ aclText }}</span>
      </div>

      <div v-if="loading" class="text-sm text-txt3">{{ t('common.buttons.loading') }}</div>

      <template v-else>
        <div class="flex items-start justify-between gap-4 pt-1 max-sm:flex-col max-sm:items-stretch">
          <div class="min-w-0">
            <strong class="block text-sm font-medium text-txt">
              {{ t('pages.projectDetail.pm.enable') }}
            </strong>
            <p class="m-0 mt-1 text-xs leading-snug text-txt3">
              {{ t('pages.projectDetail.pm.enablePermissionHint') }}
            </p>
          </div>
          <button
            type="button"
            role="switch"
            class="relative h-[22px] w-10 shrink-0 border transition max-sm:self-end disabled:cursor-not-allowed disabled:opacity-45"
            :class="
              enabled
                ? 'border-accent bg-accent'
                : 'border-line-strong bg-base'
            "
            :aria-checked="enabled"
            :aria-disabled="controlsDisabled ? 'true' : undefined"
            :aria-label="t('pages.projectDetail.pm.enable')"
            :disabled="controlsDisabled"
            :tabindex="controlsDisabled ? -1 : 0"
            @click="toggleEnabled"
            @keydown="onEnableKeydown"
          >
            <span
              class="absolute top-0.5 h-4 w-4 transition-all"
              :class="enabled ? 'left-[18px] bg-white' : 'left-0.5 bg-txt2'"
            />
          </button>
        </div>

        <div>
          <label class="label" for="pm-bind-agent">{{ t('pages.projectDetail.pm.bindAgent') }}</label>
          <select
            id="pm-bind-agent"
            v-model="selectedAgent"
            class="input max-w-md disabled:cursor-not-allowed disabled:opacity-55"
            :disabled="controlsDisabled"
          >
            <option value="">{{ t('pages.projectDetail.pm.selectAgent') }}</option>
            <option v-for="a in agentOptions" :key="a.name" :value="a.name">
              {{ a.stale ? `${a.name}${t('pages.projectDetail.pm.agentStaleSuffix')}` : a.name }}
            </option>
          </select>
          <p v-if="noAgents" class="mt-2 text-[13px] leading-snug text-err">
            {{ t('pages.projectDetail.pm.noAgents') }}
            <RouterLink to="/agents" class="ml-1 text-accent-2 underline underline-offset-2 hover:text-accent">
              {{ t('pages.projectDetail.pm.goAgents') }}
            </RouterLink>
          </p>
          <p
            v-else-if="binding?.agentError"
            class="mt-2 text-[13px] leading-snug text-err"
          >
            {{ binding.agentError }}
            <RouterLink to="/agents" class="ml-1 text-accent-2 underline underline-offset-2 hover:text-accent">
              {{ t('pages.projectDetail.pm.goAgents') }}
            </RouterLink>
          </p>
        </div>

        <div>
          <strong class="block text-sm font-medium text-txt">
            {{ t('pages.projectDetail.pm.enabledMcps') }}
          </strong>
          <p class="m-0 mt-1 text-xs leading-snug text-txt3">
            {{ t('pages.projectDetail.pm.enabledMcpsHint') }}
          </p>
          <div class="mt-2.5 flex flex-col gap-2">
            <label
              v-for="opt in PM_MCP_OPTIONS"
              :key="opt.id"
              class="flex cursor-pointer items-start gap-2.5 text-[13px] text-txt"
              :class="controlsDisabled ? 'cursor-not-allowed opacity-55' : ''"
            >
              <input
                type="checkbox"
                class="mt-0.5"
                :checked="enabledMcps.includes(opt.id)"
                :disabled="controlsDisabled"
                @change="toggleMcp(opt.id)"
              />
              <span>
                <code class="font-mono text-[12px] text-accent-2">{{ opt.id }}</code>
                <span class="mt-0.5 block text-xs text-txt3">{{ t(opt.labelKey) }}</span>
              </span>
            </label>
          </div>
        </div>

        <div class="border-t border-line pt-4">
          <div class="min-w-0">
            <strong class="block text-sm font-medium text-txt">
              {{ t('pages.projectDetail.pm.channel.sectionTitle') }}
            </strong>
            <p class="m-0 mt-1 text-xs leading-snug text-txt3">
              {{ t('pages.projectDetail.pm.channel.sectionHint') }}
            </p>
          </div>

          <label class="mt-3 flex cursor-pointer items-center gap-2.5 text-[13px] text-txt">
            <input v-model="chEnabled" type="checkbox" />
            <span>{{ t('pages.projectDetail.pm.channel.enable') }}</span>
          </label>

          <p v-if="showChannelPmWarning" class="mt-2 text-[13px] leading-snug text-warn">
            {{ t('pages.projectDetail.pm.channel.noPmLeaderWarning') }}
          </p>

          <div class="mt-3">
            <label class="label" for="ch-name">{{ t('pages.projectDetail.pm.channel.name') }}</label>
            <input
              id="ch-name"
              v-model="chName"
              class="input max-w-md"
              :placeholder="t('pages.projectDetail.pm.channel.namePlaceholder')"
            />
          </div>

          <div class="mt-3 grid grid-cols-2 gap-3 max-sm:grid-cols-1">
            <div>
              <label class="label" for="ch-appid">{{ t('pages.projectDetail.pm.channel.appId') }}</label>
              <input id="ch-appid" v-model="chAppId" class="input w-full" />
            </div>
            <div>
              <label class="label" for="ch-secret">{{ t('pages.projectDetail.pm.channel.appSecret') }}</label>
              <input
                id="ch-secret"
                v-model="chAppSecret"
                type="password"
                class="input w-full"
                :placeholder="chAppSecretSet ? t('pages.projectDetail.pm.channel.appSecretKeep') : ''"
              />
              <p v-if="chAppSecretSet" class="mt-1 text-[11px] text-txt3">
                {{ t('pages.projectDetail.pm.channel.appSecretSet') }}
              </p>
            </div>
          </div>

          <div class="mt-3">
            <label class="label" for="ch-timeout">{{ t('pages.projectDetail.pm.channel.turnTimeout') }}</label>
            <input id="ch-timeout" v-model.number="chTurnTimeout" type="number" min="0" class="input w-[140px]" />
            <p class="mt-1 text-[11px] text-txt3">{{ t('pages.projectDetail.pm.channel.turnTimeoutHint') }}</p>
          </div>

          <label class="mt-3 flex cursor-pointer items-center gap-2.5 text-[13px] text-txt">
            <input v-model="chSandbox" type="checkbox" />
            <span>{{ t('pages.projectDetail.pm.channel.sandbox') }}</span>
          </label>

          <div class="mt-3 border border-line p-3">
            <label class="flex cursor-pointer items-center gap-2.5 text-[13px] text-txt">
              <input
                v-model="chCronDeliver"
                type="checkbox"
                data-testid="cron-deliver-enable"
              />
              <span>{{ t('pages.projectDetail.pm.channel.cronDeliver') }}</span>
            </label>
            <p class="mt-1.5 text-[11px] leading-snug text-txt3">
              {{ t('pages.projectDetail.pm.channel.cronDeliverHint') }}
            </p>
            <div v-if="chCronDeliver" class="mt-3">
              <label class="label" for="ch-cron">{{ t('pages.projectDetail.pm.channel.cronDeliverTarget') }}</label>
              <div ref="targetComboRoot" class="relative">
                <div class="flex gap-1">
                  <input
                    id="ch-cron"
                    v-model="chCronDeliverTarget"
                    class="input w-full flex-1 font-mono text-[12px]"
                    placeholder="guild:123"
                    autocomplete="off"
                    role="combobox"
                    aria-autocomplete="list"
                    :aria-controls="TARGET_LISTBOX_ID"
                    :aria-expanded="targetComboOpen"
                    :aria-activedescendant="
                      targetComboOpen && targetActiveIndex >= 0
                        ? `${TARGET_LISTBOX_ID}-opt-${targetActiveIndex}`
                        : undefined
                    "
                    data-testid="cron-deliver-target-input"
                    @focus="setTargetComboOpen(true)"
                    @keydown="onTargetInputKeydown"
                  />
                  <button
                    type="button"
                    class="chip shrink-0 px-2 hover:border-accent/50"
                    data-testid="cron-deliver-target-toggle"
                    :aria-label="t('pages.projectDetail.pm.channel.recentTargetsToggle')"
                    :title="t('pages.projectDetail.pm.channel.recentTargetsToggle')"
                    :aria-expanded="targetComboOpen"
                    :aria-controls="TARGET_LISTBOX_ID"
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
                  data-testid="cron-deliver-target-listbox"
                >
                  <p
                    v-if="recentTargetsLoading && !recentTargets.length"
                    class="px-3 py-2 text-[11px] leading-4 text-txt3"
                  >
                    {{ t('common.buttons.loading') }}
                  </p>
                  <template v-else-if="recentTargets.length">
                    <button
                      v-for="(opt, idx) in recentTargets"
                      :id="`${TARGET_LISTBOX_ID}-opt-${idx}`"
                      :key="opt.value"
                      type="button"
                      class="block w-full px-3 py-2 text-left hover:bg-base"
                      :class="idx === targetActiveIndex ? 'bg-base' : ''"
                      role="option"
                      :aria-selected="idx === targetActiveIndex"
                      @click="selectPushTarget(opt.value)"
                    >
                      <span class="block truncate text-[12px] leading-snug text-txt">
                        {{ pushTargetPrimaryLabel(opt) }}
                      </span>
                      <span class="mt-0.5 block font-mono text-[11px] text-txt3">{{ opt.value }}</span>
                    </button>
                  </template>
                  <p v-else class="px-3 py-2 text-[11px] leading-4 text-txt3">
                    {{ t('pages.projectDetail.pm.channel.recentTargetsEmpty') }}
                  </p>
                </div>
              </div>
              <p class="mt-1 text-[11px] text-txt3">{{ t('pages.projectDetail.pm.channel.cronDeliverTargetHint') }}</p>
            </div>
          </div>

          <div
            class="mt-3.5 border border-line-strong bg-elevated p-3"
            data-testid="channel-session-caps"
          >
            <strong class="block text-[13px] font-semibold text-txt">
              {{ t('pages.projectDetail.pm.channel.sessionCapsTitle') }}
            </strong>
            <p class="m-0 mt-1 text-xs leading-snug text-txt3">
              {{ t('pages.projectDetail.pm.channel.sessionCapsHint') }}
            </p>

            <label
              class="mt-2.5 flex cursor-pointer items-start gap-2.5 text-[13px] text-txt select-none"
            >
              <input
                v-model="chAllowMemoryWrite"
                type="checkbox"
                class="mt-0.5"
                data-testid="channel-allow-memory-write"
              />
              <span>
                <span class="block text-[13px] text-txt">
                  {{ t('pages.projectDetail.pm.channel.allowMemoryWrite') }}
                </span>
                <span class="mt-0.5 block text-[11px] leading-snug text-txt3">
                  {{ t('pages.projectDetail.pm.channel.allowMemoryWriteHint') }}
                </span>
              </span>
            </label>

            <label
              class="mt-2.5 flex cursor-pointer items-start gap-2.5 text-[13px] text-txt select-none"
            >
              <input
                v-model="chAllowSchedulerWrite"
                type="checkbox"
                class="mt-0.5"
                data-testid="channel-allow-scheduler-write"
              />
              <span>
                <span class="block text-[13px] text-txt">
                  {{ t('pages.projectDetail.pm.channel.allowSchedulerWrite') }}
                </span>
                <span class="mt-0.5 block text-[11px] leading-snug text-txt3">
                  {{ t('pages.projectDetail.pm.channel.allowSchedulerWriteHint') }}
                </span>
              </span>
            </label>

            <div
              class="mt-3 border border-warn/35 bg-warn/10 px-2.5 py-2.5 text-xs leading-snug text-warn"
              role="note"
              data-testid="channel-session-caps-risk"
            >
              <strong class="font-semibold">{{ t('pages.projectDetail.pm.channel.sessionCapsRiskLabel') }}</strong>
              {{ t('pages.projectDetail.pm.channel.sessionCapsRisk') }}
            </div>
          </div>

          <p
            v-if="secretsKeyConfigured === false"
            class="mt-3 text-[11px] text-err"
          >
            {{ t('pages.projectDetail.pm.channel.secretKeyHint') }}
          </p>

          <div class="mt-3 flex flex-wrap justify-end gap-2">
            <AppButton v-if="channel" variant="ghost" :disabled="chSaving" @click="deleteChannel">
              {{ t('pages.projectDetail.pm.channel.delete') }}
            </AppButton>
            <AppButton variant="primary" :disabled="chSaving" @click="saveChannel">
              {{ chSaving ? t('common.buttons.saving') : t('pages.projectDetail.pm.channel.saveButton') }}
            </AppButton>
          </div>
        </div>
      </template>
    </div>

    <div class="flex shrink-0 flex-wrap justify-end gap-2 border-t border-line bg-surface p-3">
      <AppButton
        variant="primary"
        data-testid="pm-leader-save"
        :disabled="saving || noAgents || loading"
        @click="save"
      >
        {{ saving ? t('common.buttons.saving') : t('pages.projectDetail.pm.saveLeaderButton') }}
      </AppButton>
    </div>
  </div>
</template>

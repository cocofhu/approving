<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import { useToast } from '@/lib/useToast'
import type { Project, ProjectNotifyPolicy } from '@/lib/types'
import AppButton from '@/components/ui/AppButton.vue'

const props = defineProps<{
  projectId: string
  project: Project
}>()

const emit = defineEmits<{
  updated: [project: Project]
  openChannelSettings: []
}>()

const { t } = useI18n()
const toast = useToast()

const enabled = ref(true)
const waitingHuman = ref(true)
const failed = ref(true)
const hasChannel = ref(false)
const loadingChannel = ref(true)
const saving = ref(false)

function policyFromProject(p: Project): ProjectNotifyPolicy {
  const raw = p.notifyPolicy
  return {
    enabled: raw?.enabled !== false,
    defaultEvents: Array.isArray(raw?.defaultEvents)
      ? [...raw!.defaultEvents!]
      : ['waiting_human', 'failed'],
  }
}

function syncFromProject() {
  const p = policyFromProject(props.project)
  enabled.value = p.enabled !== false
  const ev = new Set(p.defaultEvents || [])
  waitingHuman.value = ev.has('waiting_human')
  failed.value = ev.has('failed')
}

async function loadChannel() {
  loadingChannel.value = true
  try {
    const res = await api.getProjectChannel(props.projectId)
    const ch = res.channel
    hasChannel.value = !!(
      ch &&
      ch.enabled &&
      String(ch.cronDeliverTarget || '').trim()
    )
  } catch {
    hasChannel.value = false
  } finally {
    loadingChannel.value = false
  }
}

watch(
  () => props.project,
  () => syncFromProject(),
  { immediate: true },
)

onMounted(() => {
  void loadChannel()
})

const statusLabel = computed(() => {
  if (!enabled.value) return t('pages.projectDetail.notify.statusOff')
  if (!hasChannel.value) return t('pages.projectDetail.notify.statusNoChannel')
  return t('pages.projectDetail.notify.statusOn')
})

async function save() {
  saving.value = true
  try {
    const defaultEvents: string[] = []
    if (waitingHuman.value) defaultEvents.push('waiting_human')
    if (failed.value) defaultEvents.push('failed')
    const notifyPolicy: ProjectNotifyPolicy = {
      enabled: enabled.value,
      defaultEvents,
    }
    const updated = await api.updateProject(props.projectId, { notifyPolicy })
    emit('updated', updated)
    toast.success(t('pages.projectDetail.notify.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    saving.value = false
  }
}

function goChannelSettings() {
  emit('openChannelSettings')
}
</script>

<template>
  <div class="mx-auto max-w-2xl" data-testid="project-notify-panel">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.notify.title') }}</h3>
        <p class="mt-1 text-[12px] text-txt3">{{ t('pages.projectDetail.notify.lead') }}</p>
      </div>
      <span
        class="inline-flex items-center rounded border px-2 py-0.5 text-[11px] font-medium"
        :class="
          !enabled
            ? 'border-err/40 bg-err/10 text-err'
            : !hasChannel
              ? 'border-warn/40 bg-warn/10 text-warn'
              : 'border-ok/40 bg-ok/10 text-ok'
        "
        data-testid="notify-status-badge"
      >
        {{ statusLabel }}
      </span>
    </div>

    <div
      v-if="!loadingChannel && !hasChannel"
      class="mb-4 rounded border border-warn/35 bg-warn/10 px-3 py-2.5 text-[12px] text-txt2"
      data-testid="notify-no-channel-hint"
    >
      {{ t('pages.projectDetail.notify.noChannelHint') }}
      <button
        type="button"
        class="ml-1 text-accent-2 underline hover:brightness-110"
        data-testid="notify-go-channel"
        @click="goChannelSettings"
      >
        {{ t('pages.projectDetail.notify.goChannel') }}
      </button>
    </div>

    <div class="space-y-4 rounded-lg border border-line bg-surface p-4">
      <label class="flex items-start gap-3">
        <button
          type="button"
          role="switch"
          class="relative mt-0.5 h-5 w-9 shrink-0 rounded-full border border-line transition"
          :class="enabled ? 'bg-accent-dim border-accent/50' : 'bg-elevated'"
          :aria-checked="enabled"
          data-testid="notify-master-toggle"
          @click="enabled = !enabled"
        >
          <span
            class="absolute top-0.5 left-0.5 h-3.5 w-3.5 rounded-full bg-txt2 transition"
            :class="enabled ? 'translate-x-4 bg-accent-2' : ''"
          />
        </button>
        <span>
          <span class="block text-sm font-medium text-txt">{{ t('pages.projectDetail.notify.master') }}</span>
          <span class="mt-0.5 block text-[12px] text-txt3">{{ t('pages.projectDetail.notify.masterHint') }}</span>
        </span>
      </label>

      <div>
        <div class="text-sm font-medium text-txt">{{ t('pages.projectDetail.notify.defaultEvents') }}</div>
        <p class="mt-0.5 text-[12px] text-txt3">{{ t('pages.projectDetail.notify.defaultEventsHint') }}</p>
        <div class="mt-2 space-y-2">
          <label class="flex items-center gap-3">
            <button
              type="button"
              role="switch"
              class="relative h-5 w-9 shrink-0 rounded-full border border-line transition"
              :class="waitingHuman ? 'bg-accent-dim border-accent/50' : 'bg-elevated'"
              :aria-checked="waitingHuman"
              data-testid="notify-ev-waiting-human"
              @click="waitingHuman = !waitingHuman"
            >
              <span
                class="absolute top-0.5 left-0.5 h-3.5 w-3.5 rounded-full bg-txt2 transition"
                :class="waitingHuman ? 'translate-x-4 bg-accent-2' : ''"
              />
            </button>
            <span class="text-[13px] text-txt">
              <code class="font-mono text-accent-2">waiting_human</code>
              <span class="ml-2 text-txt3">{{ t('pages.projectDetail.notify.evWaitingHuman') }}</span>
            </span>
          </label>
          <label class="flex items-center gap-3">
            <button
              type="button"
              role="switch"
              class="relative h-5 w-9 shrink-0 rounded-full border border-line transition"
              :class="failed ? 'bg-accent-dim border-accent/50' : 'bg-elevated'"
              :aria-checked="failed"
              data-testid="notify-ev-failed"
              @click="failed = !failed"
            >
              <span
                class="absolute top-0.5 left-0.5 h-3.5 w-3.5 rounded-full bg-txt2 transition"
                :class="failed ? 'translate-x-4 bg-accent-2' : ''"
              />
            </button>
            <span class="text-[13px] text-txt">
              <code class="font-mono text-accent-2">failed</code>
              <span class="ml-2 text-txt3">{{ t('pages.projectDetail.notify.evFailed') }}</span>
            </span>
          </label>
          <div
            class="flex items-center gap-3 opacity-50"
            data-testid="notify-ev-completed-disabled"
          >
            <button
              type="button"
              role="switch"
              class="relative h-5 w-9 shrink-0 cursor-not-allowed rounded-full border border-line bg-elevated"
              aria-checked="false"
              disabled
            >
              <span class="absolute top-0.5 left-0.5 h-3.5 w-3.5 rounded-full bg-txt3" />
            </button>
            <span class="text-[13px] text-txt3">
              <code class="font-mono">completed</code>
              <span class="ml-2">{{ t('pages.projectDetail.notify.evCompleted') }}</span>
            </span>
          </div>
        </div>
      </div>
    </div>

    <div class="mt-4 flex justify-end">
      <AppButton
        variant="primary"
        :disabled="saving"
        data-testid="notify-save"
        @click="save"
      >
        {{ saving ? t('common.buttons.saving') : t('common.buttons.save') }}
      </AppButton>
    </div>
  </div>
</template>

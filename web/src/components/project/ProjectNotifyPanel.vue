<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import type { Project, ProjectNotifyPolicy } from '@/lib/shared/types'
import {
  RUN_NOTIFY_PLACEHOLDERS,
  defaultEditableRunNotifyTemplate,
  renderRunNotifyMessage,
  type RunNotifyKind,
} from '@/lib/run/runNotifyTemplate'
import AppButton from '@/components/ui/AppButton.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'

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
const completed = ref(false)
const waitingHumanTemplate = ref('')
const failedTemplate = ref('')
const completedTemplate = ref('')
const templateKind = ref<RunNotifyKind>('waiting_human')
const hasChannel = ref(false)
const loadingChannel = ref(true)
const saving = ref(false)
const tplInput = ref<HTMLTextAreaElement | null>(null)

function policyFromProject(p: Project): ProjectNotifyPolicy {
  const raw = p.notifyPolicy
  return {
    enabled: raw?.enabled !== false,
    defaultEvents: Array.isArray(raw?.defaultEvents)
      ? [...raw!.defaultEvents!]
      : ['waiting_human', 'failed'],
    waitingHumanTemplate: raw?.waitingHumanTemplate ?? '',
    failedTemplate: raw?.failedTemplate ?? '',
    completedTemplate: raw?.completedTemplate ?? '',
  }
}

function syncFromProject() {
  const p = policyFromProject(props.project)
  enabled.value = p.enabled !== false
  const ev = new Set(p.defaultEvents || [])
  waitingHuman.value = ev.has('waiting_human')
  failed.value = ev.has('failed')
  completed.value = ev.has('completed')
  waitingHumanTemplate.value = p.waitingHumanTemplate || ''
  failedTemplate.value = p.failedTemplate || ''
  completedTemplate.value = p.completedTemplate || ''
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

const currentTemplate = computed({
  get() {
    if (templateKind.value === 'waiting_human') return waitingHumanTemplate.value
    if (templateKind.value === 'completed') return completedTemplate.value
    return failedTemplate.value
  },
  set(v: string) {
    if (templateKind.value === 'waiting_human') waitingHumanTemplate.value = v
    else if (templateKind.value === 'completed') completedTemplate.value = v
    else failedTemplate.value = v
  },
})

const previewText = computed(() =>
  renderRunNotifyMessage(templateKind.value, currentTemplate.value),
)

const previewModeLabel = computed(() => {
  const empty = !String(currentTemplate.value || '').trim()
  return empty
    ? t('pages.projectDetail.notify.previewDefault')
    : t('pages.projectDetail.notify.previewCustom')
})

const previewIsDefault = computed(() => !String(currentTemplate.value || '').trim())

async function save() {
  saving.value = true
  try {
    const defaultEvents: string[] = []
    if (waitingHuman.value) defaultEvents.push('waiting_human')
    if (failed.value) defaultEvents.push('failed')
    if (completed.value) defaultEvents.push('completed')
    // Always round-trip templates so enabled/events-only saves cannot wipe them.
    const notifyPolicy: ProjectNotifyPolicy = {
      enabled: enabled.value,
      defaultEvents,
      waitingHumanTemplate: waitingHumanTemplate.value,
      failedTemplate: failedTemplate.value,
      completedTemplate: completedTemplate.value,
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

function fillDefaultTemplate() {
  currentTemplate.value = defaultEditableRunNotifyTemplate(templateKind.value)
}

function clearTemplate() {
  currentTemplate.value = ''
}

async function insertPlaceholder(ph: string) {
  const el = tplInput.value
  if (!el) {
    currentTemplate.value = `${currentTemplate.value || ''}${ph}`
    return
  }
  const start = el.selectionStart ?? el.value.length
  const end = el.selectionEnd ?? start
  const v = el.value
  const next = v.slice(0, start) + ph + v.slice(end)
  currentTemplate.value = next
  await nextTick()
  el.focus()
  const pos = start + ph.length
  el.setSelectionRange(pos, pos)
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
        <AppSwitch
          v-model="enabled"
          class="mt-0.5"
          data-testid="notify-master-toggle"
          :aria-label="t('pages.projectDetail.notify.master')"
        />
        <span>
          <span class="block text-sm font-medium text-txt">{{ t('pages.projectDetail.notify.master') }}</span>
          <span class="mt-0.5 block text-[12px] text-txt3">{{ t('pages.projectDetail.notify.masterHint') }}</span>
        </span>
      </label>

      <div>
        <div class="text-sm font-medium text-txt">{{ t('pages.projectDetail.notify.defaultEvents') }}</div>
        <p class="mt-0.5 text-[12px] text-txt3">{{ t('pages.projectDetail.notify.defaultEventsHint') }}</p>
        <div class="mt-2 space-y-2">
          <label class="flex items-start gap-3">
            <AppSwitch
              v-model="waitingHuman"
              class="mt-0.5"
              data-testid="notify-ev-waiting-human"
              :aria-label="t('pages.projectDetail.notify.evWaitingHumanLabel')"
            />
            <span>
              <span class="block text-[13px] font-medium text-txt">{{
                t('pages.projectDetail.notify.evWaitingHumanLabel')
              }}</span>
              <span class="mt-0.5 block text-[12px] text-txt3">{{
                t('pages.projectDetail.notify.evWaitingHuman')
              }}</span>
            </span>
          </label>
          <label class="flex items-start gap-3">
            <AppSwitch
              v-model="failed"
              class="mt-0.5"
              data-testid="notify-ev-failed"
              :aria-label="t('pages.projectDetail.notify.evFailedLabel')"
            />
            <span>
              <span class="block text-[13px] font-medium text-txt">{{
                t('pages.projectDetail.notify.evFailedLabel')
              }}</span>
              <span class="mt-0.5 block text-[12px] text-txt3">{{
                t('pages.projectDetail.notify.evFailed')
              }}</span>
            </span>
          </label>
          <label class="flex items-start gap-3">
            <AppSwitch
              v-model="completed"
              class="mt-0.5"
              data-testid="notify-ev-completed"
              :aria-label="t('pages.projectDetail.notify.evCompletedLabel')"
            />
            <span>
              <span class="block text-[13px] font-medium text-txt">{{
                t('pages.projectDetail.notify.evCompletedLabel')
              }}</span>
              <span class="mt-0.5 block text-[12px] text-txt3">{{
                t('pages.projectDetail.notify.evCompleted')
              }}</span>
            </span>
          </label>
        </div>
      </div>
    </div>

    <!-- Message templates (project-level only; no workflow override) -->
    <div
      class="mt-4 space-y-3 rounded-lg border border-line bg-surface p-4"
      data-testid="notify-template-section"
    >
      <div>
        <div class="text-sm font-medium text-txt">{{ t('pages.projectDetail.notify.templatesTitle') }}</div>
        <p class="mt-0.5 text-[12px] text-txt3">{{ t('pages.projectDetail.notify.templatesHint') }}</p>
      </div>

      <div class="flex gap-2" role="tablist" data-testid="notify-template-seg">
        <button
          type="button"
          class="flex-1 rounded-md border px-3 py-2 text-[12px] transition"
          :class="
            templateKind === 'waiting_human'
              ? 'border-accent/45 bg-accent-dim text-accent-2'
              : 'border-line bg-elevated text-txt3 hover:text-txt'
          "
          data-testid="notify-tpl-seg-waiting"
          @click="templateKind = 'waiting_human'"
        >
          {{ t('pages.projectDetail.notify.segWaitingHuman') }}
        </button>
        <button
          type="button"
          class="flex-1 rounded-md border px-3 py-2 text-[12px] transition"
          :class="
            templateKind === 'failed'
              ? 'border-accent/45 bg-accent-dim text-accent-2'
              : 'border-line bg-elevated text-txt3 hover:text-txt'
          "
          data-testid="notify-tpl-seg-failed"
          @click="templateKind = 'failed'"
        >
          {{ t('pages.projectDetail.notify.segFailed') }}
        </button>
        <button
          type="button"
          class="flex-1 rounded-md border px-3 py-2 text-[12px] transition"
          :class="
            templateKind === 'completed'
              ? 'border-accent/45 bg-accent-dim text-accent-2'
              : 'border-line bg-elevated text-txt3 hover:text-txt'
          "
          data-testid="notify-tpl-seg-completed"
          @click="templateKind = 'completed'"
        >
          {{ t('pages.projectDetail.notify.segCompleted') }}
        </button>
      </div>

      <div class="flex flex-wrap items-center gap-1.5" data-testid="notify-placeholder-chips">
        <span class="mr-1 text-[11px] text-txt3">{{ t('pages.projectDetail.notify.placeholders') }}</span>
        <button
          v-for="ph in RUN_NOTIFY_PLACEHOLDERS"
          :key="ph"
          type="button"
          class="rounded border border-line bg-elevated px-1.5 py-0.5 font-mono text-[11px] text-accent-2 transition hover:border-accent/50"
          :data-testid="`notify-ph-${ph.replace(/[{}]/g, '')}`"
          @click="insertPlaceholder(ph)"
        >
          {{ ph }}
        </button>
      </div>

      <div class="flex flex-wrap gap-2">
        <AppButton
          variant="outline"
          data-testid="notify-tpl-fill-default"
          @click="fillDefaultTemplate"
        >
          {{ t('pages.projectDetail.notify.fillDefault') }}
        </AppButton>
        <AppButton
          variant="outline"
          data-testid="notify-tpl-clear"
          @click="clearTemplate"
        >
          {{ t('pages.projectDetail.notify.clearDefault') }}
        </AppButton>
      </div>

      <textarea
        ref="tplInput"
        v-model="currentTemplate"
        class="min-h-[160px] w-full resize-y rounded-md border border-line bg-elevated px-3 py-2 font-mono text-[12.5px] leading-relaxed text-txt placeholder:text-txt3 focus:border-accent/55 focus:outline-none"
        data-testid="notify-tpl-input"
        :placeholder="t('pages.projectDetail.notify.templatePlaceholder')"
        spellcheck="false"
      />

      <div data-testid="notify-preview">
        <div class="mb-2 flex items-center justify-between gap-2">
          <span class="text-[11px] uppercase tracking-wide text-txt3">
            {{ t('pages.projectDetail.notify.previewTitle') }}
          </span>
          <span
            class="rounded px-2 py-0.5 text-[11px]"
            :class="
              previewIsDefault
                ? 'bg-accent-dim/40 text-accent-2'
                : 'bg-ok/10 text-ok'
            "
            data-testid="notify-preview-mode"
          >
            {{ previewModeLabel }}
          </span>
        </div>
        <pre
          class="whitespace-pre-wrap break-all rounded-md border border-line bg-elevated px-3 py-2.5 font-mono text-[12.5px] leading-relaxed text-txt2"
          data-testid="notify-preview-body"
        >{{ previewText }}</pre>
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

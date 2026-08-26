<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import { api, type WorkspaceRevision } from '@/lib/api/api'

const props = defineProps<{
  agentName: string
  isMobile: boolean
  refreshKey?: number
}>()

const emit = defineEmits<{
  (e: 'restored'): void
}>()

const { t } = useI18n()
const revisions = ref<WorkspaceRevision[]>([])
const loading = ref(false)
const selectedSha = ref('')
const diff = ref('')
const showRestore = ref(false)
const restoreReason = ref('')
const restoring = ref(false)
const error = ref('')

const selected = computed(() => revisions.value.find((r) => r.sha === selectedSha.value))

async function loadHistory() {
  if (!props.agentName) return
  loading.value = true
  error.value = ''
  try {
    const res = await api.listAgentWorkspaceRevisions(props.agentName)
    revisions.value = res.revisions || []
    if (revisions.value.length && !selectedSha.value) {
      selectedSha.value = revisions.value[0].sha
    }
    if (selectedSha.value) await loadDiff(selectedSha.value)
  } catch (e: unknown) {
    error.value = String((e as Error)?.message || e)
  } finally {
    loading.value = false
  }
}

async function loadDiff(sha: string) {
  if (!props.agentName || !sha) return
  try {
    const res = await api.getAgentWorkspaceRevisionDiff(props.agentName, sha)
    diff.value = res.diff || ''
  } catch {
    diff.value = ''
  }
}

async function selectRevision(sha: string) {
  selectedSha.value = sha
  await loadDiff(sha)
  restoreReason.value = t('pages.agentStudio.workspaceHistory.restoreDefault', { sha: sha.slice(0, 7) })
}

function openRestore() {
  if (!selectedSha.value) return
  restoreReason.value = t('pages.agentStudio.workspaceHistory.restoreDefault', { sha: selectedSha.value.slice(0, 7) })
  showRestore.value = true
}

async function confirmRestore() {
  if (!selectedSha.value || restoring.value) return
  restoring.value = true
  error.value = ''
  try {
    await api.restoreAgentWorkspaceRevision(props.agentName, selectedSha.value, restoreReason.value.trim() || undefined)
    showRestore.value = false
    await loadHistory()
    emit('restored')
  } catch (e: unknown) {
    error.value = String((e as Error)?.message || e)
  } finally {
    restoring.value = false
  }
}

function sourceLabel(src: string) {
  const key = `pages.agentStudio.workspaceHistory.source.${src}`
  const label = t(key)
  return label === key ? src : label
}

function formatPath(rev: WorkspaceRevision) {
  const ch = rev.changes?.[0]
  if (!ch) return '—'
  if (ch.op === 'restore') return t('pages.agentStudio.workspaceHistory.restorePath')
  if (ch.op === 'baseline') return t('pages.agentStudio.workspaceHistory.baselinePath')
  return `${ch.op} ${ch.path}`
}

watch(() => props.agentName, () => {
  selectedSha.value = ''
  diff.value = ''
  void loadHistory()
}, { immediate: true })

watch(() => props.refreshKey, () => { void loadHistory() })

defineExpose({ reload: loadHistory })
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-col border-line bg-base/20" :class="isMobile ? 'border-t' : 'border-l'">
    <div class="shrink-0 border-b border-line px-2.5 py-2 text-[10.5px] font-semibold uppercase tracking-wider text-txt3">
      {{ t('pages.agentStudio.workspaceHistory.title') }}
    </div>
    <div v-if="error" class="px-2 py-1 text-[11px] text-err">{{ error }}</div>
    <div v-if="loading" class="px-3 py-4 text-[12px] text-txt3">{{ t('common.loading') }}</div>
    <div v-else class="scroll-area min-h-0 flex-1 overflow-y-auto p-2">
      <div v-if="!revisions.length" class="px-1 py-4 text-center text-[11px] leading-5 text-txt3">
        {{ t('pages.agentStudio.workspaceHistory.empty') }}
      </div>
      <button
        v-for="rev in revisions"
        :key="rev.sha"
        type="button"
        class="mb-1.5 w-full rounded-lg border px-2 py-1.5 text-left transition"
        :class="rev.sha === selectedSha ? 'border-accent/50 bg-accent-dim' : 'border-line bg-surface hover:bg-elevated'"
        @click="selectRevision(rev.sha)"
      >
        <div class="flex items-baseline justify-between gap-2">
          <span class="font-mono text-[11px] text-accent-2">{{ rev.sha.slice(0, 7) }}</span>
          <span class="chip border-line text-[10px] text-txt3">{{ sourceLabel(rev.source) }}</span>
        </div>
        <div class="mt-0.5 text-[10.5px] text-txt3">{{ rev.author }} · {{ formatPath(rev) }}</div>
        <div class="mt-1 text-[12px] text-txt2">{{ rev.reason }}</div>
      </button>
    </div>
    <div v-if="selectedSha" class="shrink-0 border-t border-line">
      <div class="border-b border-line px-2 py-1 text-[10.5px] text-txt3">
        {{ t('pages.agentStudio.workspaceHistory.diffTitle', { sha: selectedSha.slice(0, 7) }) }}
      </div>
      <pre class="scroll-area max-h-40 overflow-auto whitespace-pre-wrap p-2 font-mono text-[11px] text-txt2">{{ diff || t('pages.agentStudio.workspaceHistory.noDiff') }}</pre>
      <div class="flex gap-2 p-2">
        <AppButton size="sm" variant="outline" class="flex-1" @click="openRestore">
          {{ t('pages.agentStudio.workspaceHistory.restore') }}
        </AppButton>
      </div>
    </div>
    <AppModal
      :open="showRestore"
      :title="t('pages.agentStudio.workspaceHistory.restoreConfirmTitle')"
      :width="420"
      @close="showRestore = false"
    >
      <p class="text-[13px] leading-6 text-txt2">{{ t('pages.agentStudio.workspaceHistory.restoreConfirmBody') }}</p>
      <label class="mb-1 mt-2 block text-[12px] text-txt2">{{ t('pages.agentStudio.workspaceHistory.restoreReason') }}</label>
      <input
        v-model="restoreReason"
        class="w-full rounded border border-line bg-surface px-2 py-1.5 text-[13px] text-txt outline-none focus:border-accent"
      />
      <div class="mt-4 flex justify-end gap-2">
        <AppButton size="sm" variant="ghost" @click="showRestore = false">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton size="sm" variant="danger" :disabled="restoring" @click="confirmRestore">
          {{ restoring ? t('common.buttons.saving') : t('pages.agentStudio.workspaceHistory.restoreConfirm') }}
        </AppButton>
      </div>
    </AppModal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { api, type ProjectTaskIdentity } from '@/lib/api'
import { relTime } from '@/lib/format'
import { useToast } from '@/lib/useToast'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const toast = useToast()

const items = ref<ProjectTaskIdentity[]>([])
const loading = ref(true)
const closingId = ref('')

async function load() {
  loading.value = true
  try {
    const res = await api.listProjectTasks(props.projectId, { active: true, limit: 100 })
    items.value = res.items ?? []
  } catch (e) {
    toast.error(e instanceof Error ? e.message : t('pages.projectDetail.pm.tasksLoadFailed'))
    items.value = []
  } finally {
    loading.value = false
  }
}

async function closeTask(task: ProjectTaskIdentity, status: 'completed' | 'cancelled') {
  closingId.value = task.id
  try {
    await api.closeProjectTask(props.projectId, task.id, status)
    items.value = items.value.filter((x) => x.id !== task.id)
    toast.success(
      status === 'completed'
        ? t('pages.projectDetail.pm.tasksMarkedDone')
        : t('pages.projectDetail.pm.tasksMarkedCancelled'),
    )
  } catch (e) {
    toast.error(e instanceof Error ? e.message : t('pages.projectDetail.pm.tasksCloseFailed'))
  } finally {
    closingId.value = ''
  }
}

function isEphemeral(task: ProjectTaskIdentity) {
  return (task.runId || '').startsWith('dispatch:')
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-3 pb-4" data-testid="pm-tasks-panel">
    <div class="shrink-0 space-y-1">
      <h2 class="text-sm font-semibold text-txt">{{ t('pages.projectDetail.pm.tasksTitle') }}</h2>
      <p class="text-xs text-txt2">{{ t('pages.projectDetail.pm.tasksHint') }}</p>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-txt2" data-testid="pm-tasks-loading">
      {{ t('pages.projectDetail.pm.tasksLoading') }}
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      data-testid="pm-tasks-empty"
      :title="t('pages.projectDetail.pm.tasksEmpty')"
      :desc="t('pages.projectDetail.pm.tasksEmptyHint')"
    />

    <ul v-else class="scroll-area min-h-0 flex-1 space-y-2 overflow-y-auto" data-testid="pm-tasks-list">
      <li
        v-for="task in items"
        :key="task.id"
        class="rounded-md border border-line bg-surface px-3 py-2"
        :data-testid="`pm-task-${task.id}`"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="truncate text-sm font-medium text-txt">{{ task.shortTitle || task.id }}</span>
              <span
                class="shrink-0 rounded border border-line px-1 text-[10px] uppercase tracking-wide text-txt2"
              >{{ task.status || 'running' }}</span>
              <span
                v-if="isEphemeral(task)"
                class="shrink-0 rounded border border-amber-500/40 bg-amber-500/10 px-1 text-[10px] text-amber-700 dark:text-amber-300"
              >{{ t('pages.projectDetail.pm.tasksEphemeral') }}</span>
            </div>
            <p v-if="task.recentContext" class="line-clamp-2 text-xs text-txt2">{{ task.recentContext }}</p>
            <p class="text-[11px] text-txt3">
              <span v-if="task.originConversationId">
                {{ t('pages.projectDetail.pm.tasksConversation') }}: {{ task.originConversationId }}
                ·
              </span>
              {{ relTime(task.updatedAt) }}
            </p>
          </div>
          <div class="flex shrink-0 flex-col gap-1 sm:flex-row">
            <AppButton
              size="sm"
              variant="ghost"
              :disabled="closingId === task.id"
              :data-testid="`pm-task-done-${task.id}`"
              @click="closeTask(task, 'completed')"
            >
              {{ t('pages.projectDetail.pm.tasksMarkDone') }}
            </AppButton>
            <AppButton
              size="sm"
              variant="ghost"
              :disabled="closingId === task.id"
              :data-testid="`pm-task-cancel-${task.id}`"
              @click="closeTask(task, 'cancelled')"
            >
              {{ t('pages.projectDetail.pm.tasksMarkCancel') }}
            </AppButton>
          </div>
        </div>
      </li>
    </ul>

    <div class="shrink-0">
      <AppButton size="sm" variant="ghost" data-testid="pm-tasks-refresh" :disabled="loading" @click="load">
        {{ t('pages.projectDetail.pm.tasksRefresh') }}
      </AppButton>
    </div>
  </div>
</template>

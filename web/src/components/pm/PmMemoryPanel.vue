<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { api } from '@/lib/api'
import { useAuth } from '@/lib/useAuth'
import { useToast } from '@/lib/useToast'
import type { ProjectMemoryItem } from '@/lib/types'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const toast = useToast()
const { user } = useAuth()

const items = ref<ProjectMemoryItem[]>([])
const loading = ref(true)
const title = ref('')
const content = ref('')
const editingId = ref<string | null>(null)
const saving = ref(false)

const isAdmin = () => !!user.value?.isAdmin

async function load() {
  loading.value = true
  try {
    const res = await api.listPmMemories(props.projectId)
    items.value = res.items || []
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    loading.value = false
  }
}

function startEdit(item: ProjectMemoryItem) {
  editingId.value = item.id
  title.value = item.title
  content.value = item.content
}

function resetForm() {
  editingId.value = null
  title.value = ''
  content.value = ''
}

async function save() {
  if (!isAdmin()) return
  if (!title.value.trim()) {
    toast.error(t('pages.projectDetail.pm.memoryTitleRequired'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await api.updatePmMemory(props.projectId, editingId.value, {
        title: title.value,
        content: content.value,
      })
    } else {
      await api.upsertPmMemory(props.projectId, { title: title.value, content: content.value })
    }
    resetForm()
    await load()
    toast.success(t('pages.projectDetail.saved'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  } finally {
    saving.value = false
  }
}

async function remove(id: string) {
  if (!isAdmin()) return
  try {
    await api.deletePmMemory(props.projectId, id)
    await load()
  } catch (e: any) {
    toast.error(String(e?.message || e))
  }
}

async function clearAll() {
  if (!isAdmin()) return
  if (!confirm(t('pages.projectDetail.pm.memoryClearConfirm'))) return
  try {
    await api.clearPmMemories(props.projectId)
    await load()
    toast.success(t('pages.projectDetail.pm.memoryCleared'))
  } catch (e: any) {
    toast.error(String(e?.message || e))
  }
}

watch(
  () => props.projectId,
  () => void load(),
)
onMounted(() => void load())
</script>

<template>
  <div class="flex min-h-[420px] flex-col gap-4">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h3 class="text-base font-semibold">{{ t('pages.projectDetail.pm.memoryTitle') }}</h3>
        <p class="mt-1 text-sm text-[var(--cf-muted)]">{{ t('pages.projectDetail.pm.memoryHint') }}</p>
      </div>
      <AppButton v-if="isAdmin() && items.length" variant="ghost" @click="clearAll">
        {{ t('pages.projectDetail.pm.memoryClear') }}
      </AppButton>
    </div>

    <div v-if="isAdmin()" class="rounded-md border border-[var(--cf-border)] p-3">
      <div class="grid gap-2">
        <input
          v-model="title"
          class="rounded-md border border-[var(--cf-border)] bg-[var(--cf-surface)] px-3 py-2 text-sm"
          :placeholder="t('pages.projectDetail.pm.memoryTitlePh')"
        />
        <textarea
          v-model="content"
          rows="3"
          class="rounded-md border border-[var(--cf-border)] bg-[var(--cf-surface)] px-3 py-2 text-sm"
          :placeholder="t('pages.projectDetail.pm.memoryContentPh')"
        />
        <div class="flex gap-2">
          <AppButton :disabled="saving" @click="save">
            {{ editingId ? t('common.buttons.save') : t('pages.projectDetail.pm.memoryAdd') }}
          </AppButton>
          <AppButton v-if="editingId" variant="ghost" @click="resetForm">{{ t('common.buttons.cancel') }}</AppButton>
        </div>
      </div>
    </div>
    <p v-else class="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-100">
      {{ t('pages.projectDetail.pm.memoryReadonly') }}
    </p>

    <div v-if="loading" class="text-sm text-[var(--cf-muted)]">{{ t('common.buttons.loading') }}</div>
    <EmptyState
      v-else-if="!items.length"
      :title="t('pages.projectDetail.pm.memoryEmptyTitle')"
      :desc="t('pages.projectDetail.pm.memoryEmptyDesc')"
    />
    <ul v-else class="divide-y divide-[var(--cf-border)] rounded-md border border-[var(--cf-border)]">
      <li v-for="it in items" :key="it.id" class="px-3 py-3">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="font-medium">{{ it.title }}</div>
            <p class="mt-1 whitespace-pre-wrap text-sm text-[var(--cf-muted)]">{{ it.content }}</p>
            <p class="mt-1 text-xs text-[var(--cf-muted)]">
              {{ it.source }} · {{ it.updatedBy }} · {{ it.updatedAt }}
            </p>
          </div>
          <div v-if="isAdmin()" class="flex shrink-0 gap-1">
            <AppButton variant="ghost" size="sm" @click="startEdit(it)">{{ t('common.buttons.edit') }}</AppButton>
            <AppButton variant="ghost" size="sm" @click="remove(it.id)">{{ t('common.buttons.delete') }}</AppButton>
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>

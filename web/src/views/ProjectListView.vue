<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { api } from '@/lib/api'
import { writeStoredProjectId } from '@/lib/useProjectContext'
import { useToast } from '@/lib/useToast'
import { fmtTime } from '@/lib/format'
import { fmtCompactTokenCount } from '@/lib/tokenUsage'
import { createListRequestSeq, httpStatusOf } from '@/lib/listRequestSeq'
import type { Project } from '@/lib/types'
import TokenUsageHoverTip from '@/components/ui/TokenUsageHoverTip.vue'

const SKELETON_CARDS = 6
const listSeq = createListRequestSeq()

const router = useRouter()
const { t } = useI18n()
const toast = useToast()
const projects = ref<Project[]>([])
const loading = ref(true)
const hasInitialLoaded = ref(false)
const loadFailed = ref(false)
const loadDenied = ref(false)
const showCreate = ref(false)
const creating = ref(false)
const createName = ref('')
const createDesc = ref('')
const createError = ref('')

const showRefreshProgress = computed(
  () => loading.value && hasInitialLoaded.value && projects.value.length > 0,
)
const showSkeleton = computed(
  () => loading.value && projects.value.length === 0 && !loadFailed.value && !loadDenied.value,
)

async function load() {
  const localSeq = listSeq.beginListRequest()
  loading.value = true
  loadFailed.value = false
  loadDenied.value = false
  try {
    const data = await api.listProjects()
    if (!listSeq.isCurrentSeq(localSeq)) return
    projects.value = data
  } catch (e: unknown) {
    if (!listSeq.isCurrentSeq(localSeq)) return
    if (projects.value.length > 0) return
    const status = httpStatusOf(e)
    loadDenied.value = status === 403
    loadFailed.value = status !== 403
  } finally {
    if (!listSeq.isCurrentSeq(localSeq)) return
    loading.value = false
    hasInitialLoaded.value = true
  }
}

function openProject(p: Project) {
  writeStoredProjectId(p.id)
  router.push('/projects/' + p.id)
}

function openCreate() {
  createName.value = ''
  createDesc.value = ''
  createError.value = ''
  showCreate.value = true
}

async function confirmCreate() {
  const name = createName.value.trim()
  if (!name) {
    createError.value = t('pages.projectList.nameRequired')
    return
  }
  if (creating.value) return
  creating.value = true
  createError.value = ''
  try {
    const p = await api.createProject({ name, description: createDesc.value.trim() })
    showCreate.value = false
    toast.success(t('pages.projectList.created', { name: p.name }))
    writeStoredProjectId(p.id)
    router.push('/projects/' + p.id)
  } catch (e: any) {
    createError.value = String(e?.message || e)
  } finally {
    creating.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div>
    <div class="mb-5 flex flex-col gap-2.5 md:flex-row md:items-start md:justify-between">
      <div class="min-w-0">
        <h2 class="text-lg font-semibold text-txt">{{ t('pages.projectList.title') }}</h2>
        <p class="text-sm text-txt3">{{ t('pages.projectList.subtitle') }}</p>
      </div>
      <AppButton class="min-h-[44px] md:min-h-0" variant="primary" icon="plus" @click="openCreate">
        {{ t('pages.projectList.newProject') }}
      </AppButton>
    </div>

    <div
      data-testid="project-list-panel"
      :aria-busy="loading ? 'true' : 'false'"
    >
      <div
        v-if="showRefreshProgress"
        class="mb-2 h-[2px] overflow-hidden bg-line"
        data-testid="project-list-thin-progress"
        aria-hidden="true"
      >
        <i class="admin-list-thin-bar bg-accent" />
      </div>

      <div :class="showRefreshProgress ? 'opacity-[0.55]' : ''">
        <div
          v-if="showSkeleton"
          class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3"
          data-testid="project-list-skeleton"
          aria-hidden="true"
        >
          <div
            v-for="n in SKELETON_CARDS"
            :key="'skel-' + n"
            class="flex flex-col gap-2 rounded-lg border border-line bg-surface p-4"
          >
            <div class="flex items-start gap-3">
              <div class="h-9 w-9 shrink-0 bg-elevated animate-pulse" />
              <div class="min-w-0 flex-1 space-y-2">
                <div class="h-3.5 w-2/3 bg-elevated animate-pulse" />
                <div class="h-2.5 w-full bg-elevated animate-pulse" />
              </div>
            </div>
            <div class="border-t border-line pt-2.5">
              <div class="h-2.5 w-1/2 bg-elevated animate-pulse" />
            </div>
          </div>
        </div>

        <div
          v-else-if="loadDenied"
          role="status"
          data-testid="project-list-denied"
          class="border border-warn/40 bg-warn/10 px-5 py-10 text-center"
        >
          <Icon name="lock" :size="22" class="mx-auto mb-3 text-warn" />
          <h3 class="text-sm font-semibold text-txt">{{ t('common.asyncState.permissionDeniedTitle') }}</h3>
          <p class="mt-1 text-xs text-txt2">{{ t('common.asyncState.permissionDeniedDesc') }}</p>
          <AppButton class="mt-4" variant="outline" data-testid="project-list-retry" @click="load">
            {{ t('common.buttons.retry') }}
          </AppButton>
        </div>

        <div
          v-else-if="loadFailed"
          role="status"
          data-testid="project-list-failed"
          class="border border-err/40 bg-err/10 px-5 py-10 text-center"
        >
          <h3 class="text-sm font-semibold text-txt">{{ t('common.asyncState.loadFailedTitle') }}</h3>
          <p class="mt-1 text-xs text-txt2">{{ t('common.asyncState.loadFailedDesc') }}</p>
          <AppButton class="mt-4" variant="outline" data-testid="project-list-retry" @click="load">
            {{ t('common.buttons.retry') }}
          </AppButton>
        </div>

        <div
          v-else-if="!projects.length"
          data-testid="project-list-empty"
        >
          <EmptyState icon="folder" :title="t('pages.projectList.empty')">
            <AppButton variant="primary" icon="plus" @click="openCreate">
              {{ t('pages.projectList.newProject') }}
            </AppButton>
          </EmptyState>
        </div>

        <div v-else class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3" data-testid="project-list-cards">
          <button
            v-for="p in projects"
            :key="p.id"
            type="button"
            class="flex flex-col gap-2 rounded-lg border border-line bg-surface p-4 text-left transition hover:border-line-strong hover:bg-elevated"
            @click="openProject(p)"
          >
            <div class="flex items-start gap-3">
              <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-accent-dim text-accent-2">
                <Icon name="folder" :size="18" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-semibold text-txt">{{ p.name }}</div>
                <div v-if="p.description" class="mt-0.5 line-clamp-2 text-[12px] text-txt3">
                  {{ p.description }}
                </div>
              </div>
            </div>
            <div class="flex flex-wrap items-center border-t border-line pt-2.5 text-[11px] text-txt3">
              <span>{{ t('pages.projectList.workflowCount', { n: p.workflowCount ?? 0 }) }}</span>
              <template v-if="p.updatedAt">
                <span class="mx-1.5 text-[#d9d9d9]" aria-hidden="true">·</span>
                <span>{{ fmtTime(p.updatedAt) }}</span>
              </template>
              <span class="mx-1.5 text-[#d9d9d9]" aria-hidden="true">·</span>
              <span
                class="group relative tabular-nums"
                :class="[
                  p.totalTokens == null ? 'text-txt3' : 'text-txt2',
                  p.totalTokens != null ? 'cursor-help' : '',
                ]"
                data-testid="project-list-token"
                :aria-describedby="p.totalTokens != null ? `project-list-token-tip-${p.id}` : undefined"
              >
                {{ t('pages.projectList.tokenLabel') }}
                <em
                  class="not-italic font-bold"
                  :class="p.totalTokens == null ? 'font-semibold text-txt3' : 'text-accent-2'"
                >{{ fmtCompactTokenCount(p.totalTokens) }}</em>
                <TokenUsageHoverTip
                  v-if="p.totalTokens != null"
                  :tip-id="`project-list-token-tip-${p.id}`"
                  :total-tokens="p.totalTokens"
                  :workflow-tokens="p.workflowTokens"
                  :pm-tokens="p.pmTokens"
                />
              </span>
            </div>
          </button>
        </div>
      </div>
    </div>

    <AppModal
      :open="showCreate"
      :title="t('pages.projectList.createTitle')"
      :width="440"
      @close="!creating && (showCreate = false)"
    >
      <div class="flex flex-col gap-3">
        <label class="block text-sm">
          <span class="mb-1 block text-txt2">{{ t('pages.projectList.nameLabel') }}</span>
          <input
            v-model="createName"
            class="w-full rounded border border-line bg-elevated px-3 py-2 text-sm text-txt outline-none focus:border-accent"
            :placeholder="t('pages.projectList.namePlaceholder')"
            @keydown.enter="confirmCreate"
          />
        </label>
        <label class="block text-sm">
          <span class="mb-1 block text-txt2">{{ t('pages.projectList.descLabel') }}</span>
          <textarea
            v-model="createDesc"
            rows="2"
            class="w-full rounded border border-line bg-elevated px-3 py-2 text-sm text-txt outline-none focus:border-accent"
            :placeholder="t('pages.projectList.descPlaceholder')"
          />
        </label>
        <p v-if="createError" class="text-sm text-err">{{ createError }}</p>
        <div class="flex justify-end gap-2">
          <AppButton variant="outline" :disabled="creating" @click="showCreate = false">
            {{ t('common.buttons.cancel') }}
          </AppButton>
          <AppButton
            variant="primary"
            :disabled="creating"
            data-testid="project-list-create-submit"
            @click="confirmCreate"
          >
            {{ creating ? t('common.buttons.creating') : t('common.buttons.create') }}
          </AppButton>
        </div>
      </div>
    </AppModal>
  </div>
</template>

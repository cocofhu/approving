<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import Icon from '@/components/ui/Icon.vue'
import ReposEditor, { type RepoRow } from '@/components/ui/ReposEditor.vue'
import { api } from '@/lib/api/api'
import { useToast } from '@/lib/composables/useToast'
import type { Project } from '@/lib/shared/types'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', workflow: { id: string; name: string }): void
}>()

const router = useRouter()
const toast = useToast()
const { t } = useI18n()

type Step = 'loading' | 'error' | 'zero' | 'pick' | 'form'

const step = ref<Step>('loading')
const stepDir = ref<'fwd' | 'back'>('fwd')
const projects = ref<Project[]>([])
const loadError = ref('')
const selectedProject = ref<Project | null>(null)
const baselineName = ref('')
const baselineRepos = ref<RepoRow[]>([{ name: '', url: '', branch: '' }])
const creating = ref(false)
const createError = ref('')
const nameInput = ref<HTMLInputElement | null>(null)

const multiProject = computed(() => projects.value.length > 1)

const modalTitle = computed(() => {
  if (step.value === 'pick') return t('pages.dashboard.create.pickTitle')
  if (step.value === 'zero') return t('pages.dashboard.create.emptyTitle')
  if (step.value === 'error') return t('pages.dashboard.create.loadFailed')
  return t('pages.projectDetail.newWorkflow.modalTitle')
})

const stepHint = computed(() => {
  if (step.value === 'pick') return t('pages.dashboard.create.pickStep')
  if (step.value === 'form' && selectedProject.value) {
    return multiProject.value
      ? t('pages.dashboard.create.formStep', { name: selectedProject.value.name })
      : t('pages.dashboard.create.formProject', { name: selectedProject.value.name })
  }
  return ''
})

const canSubmit = computed(
  () =>
    baselineName.value.trim() !== '' &&
    baselineRepos.value.some((repo) => repo.url.trim() !== '') &&
    !!selectedProject.value?.id,
)

const showBack = computed(() => step.value === 'form' && multiProject.value && !creating.value)

function emptyRepos(): RepoRow[] {
  return [{ name: '', url: '', branch: '' }]
}

function applySplit() {
  const list = projects.value
  if (list.length === 0) {
    selectedProject.value = null
    step.value = 'zero'
    return
  }
  if (list.length === 1) {
    selectedProject.value = list[0]
    stepDir.value = 'fwd'
    step.value = 'form'
    return
  }
  step.value = 'pick'
}

async function loadProjects() {
  loadError.value = ''
  stepDir.value = 'fwd'
  step.value = 'loading'
  try {
    const list = await api.listProjects()
    projects.value = Array.isArray(list) ? list : []
    applySplit()
  } catch (e: any) {
    loadError.value = String(e?.message || e)
    step.value = 'error'
  }
}

function resetDraft() {
  baselineName.value = ''
  baselineRepos.value = emptyRepos()
  createError.value = ''
  creating.value = false
  selectedProject.value = null
  projects.value = []
}

function pickProject(project: Project) {
  selectedProject.value = project
  stepDir.value = 'fwd'
  step.value = 'form'
}

function goBack() {
  if (!multiProject.value || creating.value) return
  stepDir.value = 'back'
  step.value = 'pick'
}

function close() {
  if (creating.value) return
  emit('close')
}

function goProjects() {
  emit('close')
  void router.push('/projects')
}

async function submit() {
  if (!canSubmit.value || creating.value || !selectedProject.value) return
  creating.value = true
  createError.value = ''
  try {
    const created = await api.createWorkflowFromBaseline(
      selectedProject.value.id,
      baselineName.value.trim(),
      baselineRepos.value,
    )
    toast.success(t('pages.projectDetail.newWorkflow.created', { name: created.name }))
    emit('created', { id: created.id, name: created.name })
    emit('close')
  } catch (e: any) {
    createError.value = String(e?.message || e)
  } finally {
    creating.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) {
      resetDraft()
      return
    }
    resetDraft()
    void loadProjects()
  },
  { immediate: true },
)

watch(step, async (s) => {
  if (s !== 'form') return
  await nextTick()
  nameInput.value?.focus()
})
</script>

<template>
  <AppModal
    :open="open"
    :title="modalTitle"
    :width="460"
    close-on-esc
    :close-on-backdrop="!creating"
    @close="close"
  >
    <template #header>
      <div class="flex min-w-0 flex-1 items-center gap-2">
        <button
          v-if="showBack"
          type="button"
          class="home-create-back flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-txt3 hover:bg-elevated hover:text-txt"
          data-testid="home-create-back"
          :aria-label="t('pages.dashboard.create.back')"
          :title="t('pages.dashboard.create.back')"
          @click="goBack"
        >
          <Icon name="arrow-left" :size="16" />
        </button>
        <span class="truncate text-[15px] font-semibold text-txt">{{ modalTitle }}</span>
      </div>
    </template>

    <p
      v-if="stepHint"
      class="mb-3 text-[11px] leading-relaxed text-txt3"
      data-testid="home-create-steps"
    >
      {{ stepHint }}
    </p>

    <div
      :key="`${step}-${stepDir}`"
      class="home-create-step"
      :class="stepDir === 'back' ? 'home-create-step--back' : 'home-create-step--fwd'"
      data-testid="home-create-pane"
    >
      <div v-if="step === 'loading'" class="py-6 text-center text-sm text-txt3" data-testid="home-create-loading">
        {{ t('common.loading.inProgress') }}
      </div>

      <div v-else-if="step === 'error'" data-testid="home-create-load-error">
        <p class="text-[13px] leading-relaxed text-txt2">{{ t('pages.dashboard.create.loadFailed') }}</p>
        <p v-if="loadError" class="mt-2 text-[12px] text-err">{{ loadError }}</p>
      </div>

      <div v-else-if="step === 'zero'" class="py-2 text-center" data-testid="home-create-no-project">
        <p class="text-[12.5px] leading-relaxed text-txt2">{{ t('pages.dashboard.create.emptyHint') }}</p>
      </div>

      <div v-else-if="step === 'pick'" class="home-create-project-list" data-testid="home-create-project-list">
        <button
          v-for="(p, idx) in projects"
          :key="p.id"
          type="button"
          class="home-create-project-row"
          :style="{ animationDelay: `${idx * 45}ms` }"
          :data-testid="`home-create-project-${p.id}`"
          :aria-checked="selectedProject?.id === p.id ? 'true' : 'false'"
          @click="pickProject(p)"
        >
          <span class="home-create-project-av">{{ (p.name || p.id).slice(0, 1) }}</span>
          <span class="min-w-0 truncate text-[13px]">{{ p.name || p.id }}</span>
        </button>
      </div>

      <div v-else-if="step === 'form'" data-testid="home-create-form">
        <p class="mb-3 text-[13px] leading-relaxed text-txt2">
          {{ t('pages.projectDetail.newWorkflow.modalHint') }}
        </p>
        <div class="mb-4">
          <label class="label" for="home-create-workflow-name">
            {{ t('pages.projectDetail.newWorkflow.nameLabel') }}
          </label>
          <input
            id="home-create-workflow-name"
            ref="nameInput"
            v-model="baselineName"
            class="input"
            autocomplete="off"
            data-testid="home-create-workflow-name"
            :placeholder="t('pages.projectDetail.newWorkflow.namePlaceholder')"
            :disabled="creating"
          />
        </div>
        <ReposEditor :repos="baselineRepos" :min-rows="1" :editable="!creating" @update:repos="baselineRepos = $event" />
        <div
          v-if="createError"
          class="mt-3 flex items-start gap-2 rounded-md border border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err"
          role="alert"
          data-testid="home-create-error"
        >
          <Icon name="alert" :size="14" class="mt-0.5 shrink-0" />
          {{ createError }}
        </div>
      </div>
    </div>

    <template v-if="step === 'error' || step === 'zero' || step === 'form'" #footer>
      <template v-if="step === 'error'">
        <AppButton variant="ghost" @click="close">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton variant="primary" data-testid="home-create-retry" @click="loadProjects">
          {{ t('common.buttons.retry') }}
        </AppButton>
      </template>
      <template v-else-if="step === 'zero'">
        <AppButton variant="ghost" @click="close">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton variant="primary" data-testid="home-create-go-projects" @click="goProjects">
          {{ t('pages.dashboard.create.goProjects') }}
        </AppButton>
      </template>
      <template v-else>
        <AppButton variant="ghost" :disabled="creating" @click="close">{{ t('common.buttons.cancel') }}</AppButton>
        <AppButton
          variant="primary"
          :loading="creating"
          :disabled="!canSubmit"
          data-testid="home-create-submit"
          @click="submit"
        >
          {{ creating ? t('pages.projectDetail.newWorkflow.creating') : t('pages.projectDetail.newWorkflow.create') }}
        </AppButton>
      </template>
    </template>
  </AppModal>
</template>

<style scoped>
.home-create-step--fwd {
  animation: home-create-in-fwd 0.26s cubic-bezier(0.16, 1, 0.3, 1) both;
}
.home-create-step--back {
  animation: home-create-in-back 0.26s cubic-bezier(0.16, 1, 0.3, 1) both;
}
@keyframes home-create-in-fwd {
  from {
    opacity: 0;
    transform: translateX(14px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
@keyframes home-create-in-back {
  from {
    opacity: 0;
    transform: translateX(-14px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}

.home-create-project-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  text-align: left;
  border: 1px solid rgb(var(--c-line));
  background: rgb(var(--c-surface));
  border-radius: 10px;
  padding: 10px 12px;
  margin: 0 0 8px;
  cursor: pointer;
  animation: home-create-row-in 0.26s cubic-bezier(0.16, 1, 0.3, 1) both;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    opacity 0.16s ease,
    transform 0.16s cubic-bezier(0.16, 1, 0.3, 1);
}
.home-create-project-row:hover {
  border-color: rgb(var(--c-accent));
  background: rgb(var(--c-accent) / 0.08);
  transform: translateX(2px);
}
.home-create-project-row[aria-checked='true'] {
  border-color: rgb(var(--c-accent));
  background: rgb(var(--c-accent) / 0.12);
}
.home-create-project-av {
  width: 24px;
  height: 24px;
  border-radius: 7px;
  background: rgb(var(--c-elevated));
  color: rgb(var(--c-txt2));
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 600;
  flex: 0 0 auto;
}
@keyframes home-create-row-in {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}

.home-create-back {
  transition:
    background-color 0.16s ease,
    color 0.16s ease,
    transform 0.16s cubic-bezier(0.16, 1, 0.3, 1);
}

@media (prefers-reduced-motion: reduce) {
  .home-create-step--fwd,
  .home-create-step--back,
  .home-create-project-row {
    animation: none;
  }
  .home-create-project-row:hover,
  .home-create-back:active {
    transform: none;
  }
}
</style>

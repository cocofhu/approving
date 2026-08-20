<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ChatImageThumb from '@/components/ui/ChatImageThumb.vue'
import Icon from '@/components/ui/Icon.vue'
import RunLaunchModal from '@/components/workflow/RunLaunchModal.vue'
import { useHomeApproveChat } from '@/lib/run/useHomeApproveChat'
import { attachmentDisplayName, isImageAttachment } from '@/lib/shared/attachments'
import { imgSrc } from '@/lib/shared/compositeText'

const router = useRouter()
const { t } = useI18n()
const {
  projectId,
  hasProject,
  pipelines,
  selected,
  selectedId,
  draft,
  sending,
  loading,
  loadError,
  launchOpen,
  launchTarget,
  launchTitle,
  runFields,
  runInputs,
  runImages,
  draftRestored,
  attachments,
  fileInput,
  attachNotice,
  onPickFiles,
  onPaste,
  removeAttachment,
  load,
  selectPipeline,
  send,
  closeLaunch,
  onLaunchStarted,
} = useHomeApproveChat()

function goSelectProject() {
  void router.push('/projects')
}

function goProject() {
  const id = projectId.value
  if (id) {
    void router.push(`/projects/${id}`)
    return
  }
  goSelectProject()
}

function onPipelineChange(e: Event) {
  const el = e.target as HTMLSelectElement | null
  if (el) selectPipeline(el.value)
}

function onComposerSubmit(e: Event) {
  e.preventDefault()
  void send()
}

function openFilePicker() {
  fileInput.value?.click()
}
</script>

<template>
  <div data-testid="dashboard-view" class="flex flex-col md:h-full md:min-h-0">
    <div class="mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center px-4 py-10">
      <p class="text-sm font-medium tracking-wide text-txt3" data-testid="home-brand">Approving</p>
      <h2 class="mt-2 text-center text-2xl font-semibold text-txt" data-testid="home-title">
        {{ t('pages.dashboard.title') }}
      </h2>
      <p class="mt-2 max-w-xl text-center text-sm text-txt3" data-testid="home-subtitle">
        {{ t('pages.dashboard.subtitle') }}
      </p>

      <form
        class="mt-8 flex w-full flex-col gap-2 rounded-3xl border border-line bg-surface px-3 py-2"
        data-testid="home-composer"
        @submit="onComposerSubmit"
      >
        <div
          v-if="attachNotice"
          class="border border-err/40 bg-err/10 px-2.5 py-1.5 text-[12px] text-err"
          data-testid="home-attach-notice"
          role="alert"
        >
          {{ attachNotice.text }}
        </div>
        <div v-if="attachments.length" class="flex flex-wrap gap-1.5" data-testid="home-pending-attachments">
          <div v-for="(im, ii) in attachments" :key="ii" class="relative">
            <ChatImageThumb
              v-if="isImageAttachment(im)"
              mode="locked"
              size="sm"
              thumb-class="rounded-md"
              :src="imgSrc(im)"
              :label="attachmentDisplayName(im, ii)"
              :alt="attachmentDisplayName(im, ii)"
              test-id="home-draft-image-thumb"
            />
            <div
              v-else
              class="flex h-14 max-w-[160px] items-center gap-1.5 border border-line bg-elevated px-2"
              :title="attachmentDisplayName(im, ii)"
              data-testid="home-pending-file-chip"
            >
              <span class="shrink-0 text-[9px] font-medium uppercase text-info">DOC</span>
              <span class="min-w-0 truncate text-[11px] text-txt2">{{ attachmentDisplayName(im, ii) }}</span>
            </div>
            <button
              type="button"
              class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-err text-white disabled:opacity-40"
              :disabled="sending"
              data-testid="home-remove-attachment"
              @click.stop="removeAttachment(ii)"
            >
              <Icon name="close" :size="9" />
            </button>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <input
            ref="fileInput"
            type="file"
            multiple
            class="hidden"
            data-testid="home-attach-input"
            @change="onPickFiles"
          />
          <button
            type="button"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-txt3 hover:bg-elevated disabled:opacity-40"
            :disabled="sending"
            :title="t('pages.dashboard.addAttachment')"
            :aria-label="t('pages.dashboard.addAttachment')"
            data-testid="home-composer-plus"
            @click="openFilePicker"
          >
            <Icon name="plus" :size="16" />
          </button>
          <label class="sr-only" for="home-pipeline-select">{{ t('pages.dashboard.pickPipeline') }}</label>
          <select
            id="home-pipeline-select"
            class="max-w-[10rem] shrink-0 cursor-pointer appearance-none rounded-full bg-accent/15 px-3 py-1 text-xs font-medium text-accent outline-none disabled:cursor-default disabled:bg-elevated disabled:text-txt3"
            data-testid="home-pipeline-select"
            :disabled="!pipelines.length || sending"
            :value="selectedId"
            @change="onPipelineChange"
          >
            <option v-if="!pipelines.length" value="">{{ t('pages.dashboard.noPipelineShort') }}</option>
            <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
          <input
            v-model="draft"
            class="min-w-0 flex-1 bg-transparent px-1 text-sm text-txt outline-none placeholder:text-txt3"
            data-testid="home-composer-input"
            :placeholder="t('pages.dashboard.placeholder')"
            :disabled="sending"
            autocomplete="off"
            @paste="onPaste"
          />
          <button
            type="submit"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-txt text-base disabled:opacity-40"
            data-testid="home-composer-send"
            :disabled="sending"
            :aria-label="t('pages.dashboard.send')"
          >
            <Icon name="arrow-up" :size="16" />
          </button>
        </div>
      </form>
      <p class="mt-2 text-center text-[12px] text-txt3" data-testid="home-filter-hint">
        {{ t('pages.dashboard.filterHint') }}
      </p>

      <div
        v-if="loadError"
        class="mt-6 flex w-full flex-wrap items-center justify-between gap-2 border border-err/40 bg-err/10 px-3 py-2 text-[13px] text-err"
        data-testid="dashboard-load-error"
      >
        <span>{{ t('pages.board.loadFailed') }}</span>
        <button
          type="button"
          class="border border-err/40 px-2.5 py-1 text-xs text-err hover:bg-err/10"
          data-testid="dashboard-retry"
          @click="load()"
        >
          {{ t('pages.board.retry') }}
        </button>
      </div>

      <div v-else-if="!hasProject" class="mt-10 text-center" data-testid="home-no-project">
        <p class="text-sm text-txt3">{{ t('pages.dashboard.noProject') }}</p>
        <button
          type="button"
          class="mt-3 border border-line px-3 py-1.5 text-[13px] text-txt2 hover:bg-elevated"
          data-testid="dashboard-select-project"
          @click="goSelectProject"
        >
          {{ t('pages.dashboard.selectProject') }}
        </button>
      </div>

      <div v-else-if="loading" class="mt-10 text-sm text-txt3" data-testid="home-pipelines-loading">
        {{ t('pages.board.loading') }}
      </div>

      <div v-else-if="!pipelines.length" class="mt-10 text-center" data-testid="home-pipelines-empty">
        <p class="text-sm text-txt3">{{ t('pages.dashboard.noPipelines') }}</p>
        <button
          type="button"
          class="mt-3 border border-line px-3 py-1.5 text-[13px] text-txt2 hover:bg-elevated"
          data-testid="home-go-canvas"
          @click="goProject"
        >
          {{ t('pages.dashboard.goCanvas') }}
        </button>
      </div>

      <div v-else class="mt-8 flex w-full gap-3 overflow-x-auto pb-2" data-testid="home-pipeline-cards">
        <button
          v-for="p in pipelines"
          :key="p.id"
          type="button"
          class="card w-48 shrink-0 overflow-hidden p-0 text-left transition"
          :class="p.id === selected?.id ? 'border-accent ring-1 ring-accent/40' : 'hover:border-line-strong'"
          :data-testid="`home-pipeline-card-${p.id}`"
          @click="selectPipeline(p.id)"
        >
          <div class="flex h-20 items-center justify-center bg-elevated">
            <span class="flex items-center gap-1.5">
              <span class="h-2 w-2 rounded-full bg-txt3" />
              <span class="h-px w-6 bg-line-strong" />
              <span class="h-2.5 w-2.5 rounded-full bg-accent" />
              <span class="h-px w-6 bg-line-strong" />
              <span class="h-2 w-2 rounded-full bg-txt3" />
            </span>
          </div>
          <div class="px-3 py-2.5">
            <div class="truncate text-[13px] font-medium text-txt">{{ p.name }}</div>
            <div class="mt-0.5 line-clamp-2 text-[11px] text-txt3">
              {{ p.description || t('pages.dashboard.cardFallback') }}
            </div>
          </div>
        </button>
      </div>
    </div>

    <RunLaunchModal
      :open="launchOpen"
      :workflow-id="launchTarget?.id || ''"
      :project-id="projectId"
      :workflow-name="launchTarget?.name || ''"
      :fields="runFields"
      :run-inputs="runInputs"
      :run-images="runImages"
      :draft-restored="draftRestored"
      :run-title="launchTitle"
      @close="closeLaunch()"
      @stayed="closeLaunch()"
      @started="onLaunchStarted($event)"
    />
  </div>
</template>

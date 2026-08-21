<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import ChatImageThumb from '@/components/ui/ChatImageThumb.vue'
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
  canSend,
  loading,
  loadError,
  launchOpen,
  launchTarget,
  launchTitle,
  launchFirstMessage,
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
  <div data-testid="dashboard-view" class="home-stage relative flex flex-col md:h-full md:min-h-0">
    <div class="home-stage__bg" aria-hidden="true" data-testid="home-stage-bg">
      <div class="home-stage__wash" />
      <div class="home-stage__grid" />
      <div class="home-stage__glow" />
    </div>

    <div
      class="home-stage__content relative z-[1] mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center px-4 py-10"
    >
      <p class="home-stage__brand" data-testid="home-brand">Approving</p>
      <h2 class="home-stage__headline mt-5 text-center" data-testid="home-title">
        {{ t('pages.dashboard.title') }}
      </h2>
      <p class="mt-2.5 max-w-xl text-center text-[13px] text-txt2" data-testid="home-subtitle">
        {{ t('pages.dashboard.subtitle') }}
      </p>

      <div class="mt-8 w-full">
        <p
          v-if="attachNotice"
          class="mb-2 border border-err/40 bg-err/10 px-3 py-1.5 text-[12px] text-err"
          data-testid="home-attach-notice"
          role="alert"
        >
          {{ attachNotice.text }}
        </p>
        <div
          v-if="attachments.length"
          class="mb-2 flex flex-wrap gap-1.5"
          data-testid="home-attach-chips"
        >
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
              <span class="min-w-0 truncate text-[11px] text-txt2">{{
                attachmentDisplayName(im, ii)
              }}</span>
            </div>
            <button
              type="button"
              class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-err text-white"
              data-testid="home-attach-remove"
              @click.stop="removeAttachment(ii)"
            >
              <Icon name="close" :size="9" />
            </button>
          </div>
        </div>
        <form
          class="home-stage__composer flex w-full items-center gap-2 border px-3 py-2"
          data-testid="home-composer"
          @submit="onComposerSubmit"
        >
          <input
            ref="fileInput"
            type="file"
            multiple
            class="hidden"
            data-testid="home-composer-file"
            @change="onPickFiles"
          />
          <button
            type="button"
            class="flex h-8 w-8 shrink-0 items-center justify-center text-txt3 hover:text-txt disabled:opacity-40"
            :disabled="sending"
            :title="t('pages.clarify.addImage')"
            data-testid="home-composer-plus"
            @click="openFilePicker"
          >
            <Icon name="plus" :size="16" />
          </button>
          <label class="sr-only" for="home-pipeline-select">{{ t('pages.dashboard.pickPipeline') }}</label>
          <select
            id="home-pipeline-select"
            class="home-stage__pipeline max-w-[10rem] shrink-0 cursor-pointer appearance-none border bg-accent/15 px-3 py-1.5 text-xs font-medium text-accent outline-none disabled:cursor-default disabled:bg-elevated disabled:text-txt3"
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
            class="home-stage__input min-w-0 flex-1 bg-transparent px-1 text-sm text-txt outline-none placeholder:text-txt3"
            data-testid="home-composer-input"
            :placeholder="t('pages.dashboard.placeholder')"
            :disabled="sending"
            autocomplete="off"
            @paste="onPaste"
          />
          <button
            type="submit"
            class="home-stage__send flex h-8 w-8 shrink-0 items-center justify-center text-base disabled:opacity-40"
            data-testid="home-composer-send"
            :disabled="sending || !canSend"
            :aria-label="t('pages.dashboard.send')"
          >
            <Icon name="arrow-up" :size="16" />
          </button>
        </form>
      </div>
      <p class="mt-3 text-center text-[11px] text-txt3" data-testid="home-filter-hint">
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
          class="card home-stage__card w-48 shrink-0 overflow-hidden p-0 text-left transition"
          :class="p.id === selected?.id ? 'border-accent ring-1 ring-accent/40' : 'hover:border-line-strong'"
          :data-testid="`home-pipeline-card-${p.id}`"
          @click="selectPipeline(p.id)"
        >
          <div class="flex h-20 items-center justify-center bg-elevated/80">
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
      :first-message="launchFirstMessage"
      @close="closeLaunch()"
      @stayed="closeLaunch()"
      @started="onLaunchStarted($event)"
    />
  </div>
</template>

<style scoped>
/* g1.1 — edge-to-edge stage wash / grid / glow (dark + light readable) */
.home-stage {
  isolation: isolate;
}

.home-stage__bg {
  pointer-events: none;
  position: absolute;
  inset: 0;
  z-index: 0;
  overflow: hidden;
}

.home-stage__wash {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse 90% 55% at 50% 8%, rgba(91, 66, 180, 0.48) 0%, transparent 58%),
    radial-gradient(ellipse 60% 40% at 80% 70%, rgba(49, 46, 129, 0.28) 0%, transparent 55%),
    linear-gradient(180deg, rgba(10, 10, 11, 0.12) 0%, rgba(10, 10, 11, 0.55) 55%, rgb(var(--c-base)) 100%),
    linear-gradient(135deg, rgb(17 24 39) 0%, rgb(var(--c-base)) 42%, rgb(30 27 75) 100%);
}

:global(html.light) .home-stage__wash {
  background:
    radial-gradient(ellipse 90% 55% at 50% 8%, rgba(123, 97, 255, 0.22) 0%, transparent 58%),
    radial-gradient(ellipse 55% 38% at 82% 72%, rgba(99, 102, 241, 0.14) 0%, transparent 55%),
    linear-gradient(180deg, rgba(250, 250, 251, 0.35) 0%, rgba(250, 250, 251, 0.82) 55%, rgb(var(--c-base)) 100%),
    linear-gradient(135deg, #eef0ff 0%, rgb(var(--c-base)) 45%, #e8e7f8 100%);
}

.home-stage__grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.045) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.045) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(circle at 50% 28%, #000 18%, transparent 72%);
  -webkit-mask-image: radial-gradient(circle at 50% 28%, #000 18%, transparent 72%);
  animation: home-stage-drift 18s linear infinite;
}

:global(html.light) .home-stage__grid {
  background-image:
    linear-gradient(rgba(99, 102, 241, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(99, 102, 241, 0.08) 1px, transparent 1px);
}

.home-stage__glow {
  position: absolute;
  top: 10%;
  left: 50%;
  width: min(720px, 90vw);
  height: 220px;
  transform: translateX(-50%);
  background: radial-gradient(ellipse at center, rgba(167, 139, 250, 0.28), transparent 70%);
  filter: blur(8px);
  animation: home-stage-pulse 5.5s ease-in-out infinite;
}

:global(html.light) .home-stage__glow {
  background: radial-gradient(ellipse at center, rgba(123, 97, 255, 0.18), transparent 70%);
}

/* g1.2 — brand as first visual anchor */
.home-stage__brand {
  font-size: clamp(2.75rem, 9vw, 5.5rem);
  font-weight: 700;
  letter-spacing: -0.04em;
  line-height: 0.92;
  color: rgb(var(--c-accent-2));
  background: var(--grad-logo);
  background-size: var(--grad-logo-size);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  animation: home-stage-brand-in 0.9s ease-out both, shimmer 3.5s ease-in-out 0.9s infinite;
}

.home-stage__headline {
  font-size: clamp(1.25rem, 3.2vw, 1.75rem);
  font-weight: 600;
  letter-spacing: -0.02em;
  color: rgb(var(--c-txt));
}

/* g1.3 — floating composer on stage plane */
.home-stage__composer {
  border-color: rgba(196, 181, 253, 0.35);
  background: rgba(10, 10, 11, 0.58);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow:
    0 24px 60px rgba(0, 0, 0, 0.35),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

:global(html.light) .home-stage__composer {
  border-color: rgba(123, 97, 255, 0.28);
  background: rgba(255, 255, 255, 0.78);
  box-shadow:
    0 18px 40px rgba(16, 24, 40, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.9);
}

.home-stage__pipeline {
  border-color: rgba(196, 181, 253, 0.45);
}

:global(html.light) .home-stage__pipeline {
  border-color: rgba(123, 97, 255, 0.35);
}

.home-stage__send {
  background: rgb(237 233 254);
  color: #111;
}

:global(html.light) .home-stage__send {
  background: rgb(var(--c-txt));
  color: rgb(var(--c-base));
}

.home-stage__card {
  background: rgba(20, 20, 23, 0.72);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

:global(html.light) .home-stage__card {
  background: rgba(255, 255, 255, 0.82);
}

@keyframes home-stage-drift {
  from {
    transform: translateY(0);
    opacity: 0.9;
  }
  to {
    transform: translateY(24px);
    opacity: 0.75;
  }
}

@keyframes home-stage-pulse {
  0%,
  100% {
    opacity: 0.55;
  }
  50% {
    opacity: 0.9;
  }
}

@keyframes home-stage-brand-in {
  from {
    opacity: 0;
    transform: translateY(12px);
    filter: blur(4px);
  }
  to {
    opacity: 1;
    transform: none;
    filter: none;
  }
}

/* g2.3 — narrow screens keep brand anchor + operable composer */
@media (max-width: 520px) {
  .home-stage__content {
    padding-top: 2rem;
    padding-bottom: 2.5rem;
    justify-content: flex-start;
  }

  .home-stage__composer {
    flex-wrap: wrap;
  }

  .home-stage__input {
    min-width: 100%;
    order: 3;
  }
}
</style>

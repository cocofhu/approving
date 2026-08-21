<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import ChatImageThumb from '@/components/ui/ChatImageThumb.vue'
import RunLaunchModal from '@/components/workflow/RunLaunchModal.vue'
import { useHomeApproveChat } from '@/lib/run/useHomeApproveChat'
import { attachmentDisplayName, isImageAttachment } from '@/lib/shared/attachments'
import { imgSrc } from '@/lib/shared/compositeText'

const BRAND_TEXT = 'Approving'

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

const brandVisible = ref('')
const brandCursor = ref(false)
const brandTimers: ReturnType<typeof setTimeout>[] = []
const composerFocused = ref(false)
const composing = ref(false)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const overflowScroll = ref(false)
const phVisible = ref('')
const phCursor = ref(false)
let phTimer: ReturnType<typeof setTimeout> | null = null
let phLoopTimer: ReturnType<typeof setTimeout> | null = null

const placeholderFull = computed(() => t('pages.dashboard.placeholder'))
const showPhTypewriter = computed(() => !draft.value.trim() && !composerFocused.value)

function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function clearBrandTimers() {
  while (brandTimers.length) {
    const id = brandTimers.pop()
    if (id != null) clearTimeout(id)
  }
}

function scheduleBrand(fn: () => void, ms: number) {
  brandTimers.push(setTimeout(fn, ms))
}

function runBrandTypewriter() {
  clearBrandTimers()
  if (prefersReducedMotion()) {
    brandVisible.value = BRAND_TEXT
    brandCursor.value = false
    return
  }
  brandVisible.value = ''
  brandCursor.value = true
  let i = 0
  const typeNext = () => {
    if (i < BRAND_TEXT.length) {
      i += 1
      brandVisible.value = BRAND_TEXT.slice(0, i)
      scheduleBrand(typeNext, 72)
      return
    }
    // Soft blink a few times, then hide the caret permanently.
    let blinks = 0
    const blink = () => {
      brandCursor.value = !brandCursor.value
      blinks += 1
      if (blinks < 6) {
        scheduleBrand(blink, 380)
        return
      }
      brandCursor.value = false
    }
    scheduleBrand(blink, 420)
  }
  scheduleBrand(typeNext, 120)
}

function clearPhTimers() {
  if (phTimer != null) {
    clearTimeout(phTimer)
    phTimer = null
  }
  if (phLoopTimer != null) {
    clearTimeout(phLoopTimer)
    phLoopTimer = null
  }
}

function runPlaceholderTypewriter() {
  clearPhTimers()
  const full = placeholderFull.value
  if (!showPhTypewriter.value) {
    phVisible.value = ''
    phCursor.value = false
    return
  }
  if (prefersReducedMotion()) {
    phVisible.value = full
    phCursor.value = false
    return
  }
  phVisible.value = ''
  phCursor.value = true
  let i = 0
  const typeNext = () => {
    if (!showPhTypewriter.value) return
    if (i < full.length) {
      i += 1
      phVisible.value = full.slice(0, i)
      phTimer = setTimeout(typeNext, 64)
      return
    }
    // Hold, then briefly blink caret and restart once idle.
    phLoopTimer = setTimeout(() => {
      if (!showPhTypewriter.value) return
      phCursor.value = false
      phLoopTimer = setTimeout(() => {
        if (showPhTypewriter.value) runPlaceholderTypewriter()
      }, 900)
    }, 2200)
  }
  phTimer = setTimeout(typeNext, 80)
}

function autoGrow() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const h = Math.min(Math.max(el.scrollHeight, 56), 200)
  el.style.height = `${h}px`
  overflowScroll.value = el.scrollHeight > 200
}

function onTextInput() {
  autoGrow()
}

function onComposerKeydown(e: KeyboardEvent) {
  if (e.key !== 'Enter' || e.shiftKey) return
  if (composing.value || e.isComposing || e.keyCode === 229) return
  e.preventDefault()
  void send()
}

function onComposerFocus() {
  composerFocused.value = true
}

function onComposerBlur() {
  composerFocused.value = false
}

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

watch(draft, () => nextTick(autoGrow))
watch(showPhTypewriter, () => runPlaceholderTypewriter(), { immediate: true })
watch(placeholderFull, () => {
  if (showPhTypewriter.value) runPlaceholderTypewriter()
})

onMounted(() => {
  runBrandTypewriter()
  nextTick(autoGrow)
})

onBeforeUnmount(() => {
  clearBrandTimers()
  clearPhTimers()
})
</script>

<template>
  <div data-testid="dashboard-view" class="home-shell relative flex flex-col md:h-full md:min-h-0">
    <div
      class="home-shell__content relative z-[1] mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center px-4 py-10"
    >
      <h1 class="home-brand" data-testid="home-brand" aria-label="Approving">
        <span class="home-brand__text" data-testid="home-brand-text">{{ brandVisible }}</span>
        <span
          v-if="brandCursor"
          class="home-brand__cursor"
          data-testid="home-brand-cursor"
          aria-hidden="true"
        >▌</span>
      </h1>
      <p class="home-hint mt-4 text-center" data-testid="home-title">
        {{ t('pages.dashboard.title') }}
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
              thumb-class="rounded-none"
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
          class="home-composer flex w-full flex-col border"
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
          <div class="home-composer__field relative px-3 pt-3">
            <label class="sr-only" for="home-composer-input">{{ t('pages.dashboard.placeholder') }}</label>
            <textarea
              id="home-composer-input"
              ref="textareaRef"
              v-model="draft"
              class="home-composer__input w-full min-w-0 resize-none bg-transparent text-sm text-txt outline-none"
              :class="overflowScroll ? 'scroll-area max-h-[200px] overflow-y-auto' : 'overflow-y-hidden'"
              data-testid="home-composer-input"
              rows="2"
              :disabled="sending"
              autocomplete="off"
              @input="onTextInput"
              @keydown="onComposerKeydown"
              @compositionstart="composing = true"
              @compositionend="composing = false"
              @paste="onPaste"
              @focus="onComposerFocus"
              @blur="onComposerBlur"
            />
            <span
              v-if="showPhTypewriter"
              class="home-composer__ph pointer-events-none absolute left-3 top-3 text-sm text-txt3"
              data-testid="home-composer-placeholder"
              aria-hidden="true"
            >
              {{ phVisible }}<span
                v-if="phCursor"
                class="home-composer__ph-cursor"
                data-testid="home-composer-ph-cursor"
              >▌</span>
            </span>
          </div>
          <div class="home-composer__toolbar flex items-center gap-2 border-t px-2 py-2">
            <button
              type="button"
              class="home-composer__plus flex h-9 w-9 shrink-0 items-center justify-center border text-txt2 hover:text-txt disabled:opacity-40"
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
              class="home-composer__pipeline max-w-[12rem] min-w-0 flex-1 cursor-pointer appearance-none border bg-transparent px-2.5 py-1.5 text-xs font-medium text-txt outline-none disabled:cursor-default disabled:text-txt3"
              data-testid="home-pipeline-select"
              :disabled="!pipelines.length || sending"
              :value="selectedId"
              @change="onPipelineChange"
            >
              <option v-if="!pipelines.length" value="">{{ t('pages.dashboard.noPipelineShort') }}</option>
              <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
            <button
              type="submit"
              class="home-composer__send flex h-9 w-9 shrink-0 items-center justify-center text-base disabled:opacity-40"
              data-testid="home-composer-send"
              :disabled="sending || !canSend"
              :aria-label="t('pages.dashboard.send')"
            >
              <Icon name="arrow-up" :size="16" />
            </button>
          </div>
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
          class="card home-shell__card rounded-none w-48 shrink-0 overflow-hidden p-0 text-left transition"
          :class="p.id === selected?.id ? 'border-accent ring-1 ring-accent/40' : 'hover:border-line-strong'"
          :data-testid="`home-pipeline-card-${p.id}`"
          @click="selectPipeline(p.id)"
        >
          <div class="flex h-20 items-center justify-center bg-elevated/80">
            <span class="flex items-center gap-1.5">
              <span class="h-2 w-2 bg-txt3" />
              <span class="h-px w-6 bg-line-strong" />
              <span class="h-2.5 w-2.5 bg-accent" />
              <span class="h-px w-6 bg-line-strong" />
              <span class="h-2 w-2 bg-txt3" />
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
/* g1 — clean shell, no full-bleed purple stage */
.home-shell {
  isolation: isolate;
}

/* g1.3 — monospace brand; solid color (no gradient flash / purple haze) */
.home-brand {
  display: inline-flex;
  align-items: baseline;
  margin: 0;
  font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: clamp(2.5rem, 8vw, 4.75rem);
  font-weight: 600;
  letter-spacing: -0.045em;
  line-height: 0.95;
  color: rgb(var(--c-txt));
}

:global(html.light) .home-brand {
  color: rgb(var(--c-txt));
}

.home-brand__cursor {
  display: inline-block;
  margin-left: 0.06em;
  font-weight: 400;
  color: rgb(var(--c-accent-2));
  animation: home-brand-caret 0.76s step-end infinite;
}

.home-hint {
  font-size: clamp(0.95rem, 2.4vw, 1.125rem);
  font-weight: 500;
  letter-spacing: -0.01em;
  color: rgb(var(--c-txt2));
}

/* g1.4 / g2.4 — right-angle Open Design composer */
.home-composer {
  border-color: rgb(var(--c-line-strong));
  background: rgb(var(--c-surface));
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.22);
}

:global(html.light) .home-composer {
  border-color: rgb(var(--c-line));
  background: rgb(var(--c-elevated));
  box-shadow: 0 10px 28px rgba(16, 24, 40, 0.06);
}

.home-composer__toolbar {
  border-color: rgb(var(--c-line));
  background: rgb(var(--c-elevated) / 0.55);
}

:global(html.light) .home-composer__toolbar {
  background: rgb(var(--c-base) / 0.45);
}

.home-composer__input {
  min-height: 56px;
  line-height: 1.5;
}

.home-composer__ph-cursor {
  display: inline-block;
  margin-left: 1px;
  color: rgb(var(--c-txt3));
  animation: home-brand-caret 0.76s step-end infinite;
}

.home-composer__plus {
  border-color: rgb(var(--c-line));
  background: transparent;
  transition: border-color 0.15s ease, color 0.15s ease, background-color 0.15s ease;
}

.home-composer__plus:hover:not(:disabled) {
  border-color: rgb(var(--c-line-strong));
  background: rgb(var(--c-elevated));
}

.home-composer__pipeline {
  border-color: rgb(var(--c-line));
  color: rgb(var(--c-txt));
}

.home-composer__pipeline:focus {
  border-color: rgb(var(--c-accent-2));
}

.home-composer__send {
  background: rgb(var(--c-txt));
  color: rgb(var(--c-base));
  transition: opacity 0.15s ease, background-color 0.15s ease;
}

.home-composer__send:hover:not(:disabled) {
  background: rgb(var(--c-accent-2));
  color: #fff;
}

:global(html.light) .home-composer__send:hover:not(:disabled) {
  background: rgb(var(--c-txt));
  color: rgb(var(--c-base));
}

.home-shell__card {
  background: rgb(var(--c-surface));
  border-radius: 0;
}

:global(html.light) .home-shell__card {
  background: rgb(var(--c-elevated));
}

@keyframes home-brand-caret {
  50% {
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-brand__cursor,
  .home-composer__ph-cursor {
    animation: none;
    opacity: 1;
  }
}

@media (max-width: 520px) {
  .home-shell__content {
    padding-top: 2rem;
    padding-bottom: 2.5rem;
    justify-content: flex-start;
  }

  .home-composer__pipeline {
    max-width: none;
  }
}
</style>

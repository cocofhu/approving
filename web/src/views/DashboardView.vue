<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import HomeParticleMeshBackground from '@/components/dashboard/HomeParticleMeshBackground.vue'
import Icon from '@/components/ui/Icon.vue'
import ChatImageThumb from '@/components/ui/ChatImageThumb.vue'
import ChatImagePreviewModal from '@/components/ui/ChatImagePreviewModal.vue'
import RunLaunchModal from '@/components/workflow/RunLaunchModal.vue'
import { useChatImagePreview } from '@/lib/composables/useChatImagePreview'
import { useHomeApproveChat } from '@/lib/run/useHomeApproveChat'
import { attachmentDisplayName, isImageAttachment } from '@/lib/shared/attachments'
import { imgSrc } from '@/lib/shared/compositeText'

const { preview: imagePreview, openChatImagePreview, closeChatImagePreview } = useChatImagePreview()

const BRAND_TEXT = 'Approving'

const router = useRouter()
const { t, tm } = useI18n()
const {
  projectId,
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
/** Keep caret in layout; hide with opacity so settle does not shift the centered brand. */
const brandCursorGone = ref(false)
const brandCursorBlink = ref(false)
const brandTimers: ReturnType<typeof setTimeout>[] = []
const composerFocused = ref(false)
const composing = ref(false)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const overflowScroll = ref(false)
const phVisible = ref('')
const phCursor = ref(false)
let phTimer: ReturnType<typeof setTimeout> | null = null
let phHoldTimer: ReturnType<typeof setTimeout> | null = null

const placeholderLines = computed(() => {
  const raw = tm('pages.dashboard.placeholders') as unknown
  if (Array.isArray(raw) && raw.length > 0) {
    const lines = raw.filter((s): s is string => typeof s === 'string' && s.trim().length > 0)
    if (lines.length > 0) return lines
  }
  const single = String(t('pages.dashboard.placeholder')).trim()
  return single ? [single] : []
})
const placeholderPrimary = computed(() => placeholderLines.value[0] ?? '')
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

/** Monospace brand: type once, soft blink caret, then opacity-hide caret (keep box). */
function runBrandTypewriter() {
  clearBrandTimers()
  brandCursorBlink.value = false
  brandCursorGone.value = false
  if (prefersReducedMotion()) {
    brandVisible.value = BRAND_TEXT
    brandCursorGone.value = true
    return
  }
  brandVisible.value = ''
  let i = 0
  const typeNext = () => {
    if (i < BRAND_TEXT.length) {
      i += 1
      brandVisible.value = BRAND_TEXT.slice(0, i)
      scheduleBrand(typeNext, 78)
      return
    }
    brandCursorBlink.value = true
    scheduleBrand(() => {
      brandCursorBlink.value = false
      brandCursorGone.value = true
    }, 850 * 3)
  }
  scheduleBrand(typeNext, 220)
}

function clearPhTimers() {
  if (phTimer != null) {
    clearTimeout(phTimer)
    phTimer = null
  }
  if (phHoldTimer != null) {
    clearTimeout(phHoldTimer)
    phHoldTimer = null
  }
}

/** g1.2 — idle placeholder typewriter (type → hold → delete → next line → repeat). */
function runPlaceholderTypewriter() {
  clearPhTimers()
  const lines = placeholderLines.value
  if (!showPhTypewriter.value) {
    phVisible.value = ''
    phCursor.value = false
    return
  }
  const first = lines[0] ?? ''
  if (prefersReducedMotion()) {
    phVisible.value = first
    phCursor.value = true
    return
  }
  phVisible.value = ''
  phCursor.value = true
  let li = 0
  let i = 0
  let deleting = false
  const tick = () => {
    if (!showPhTypewriter.value) return
    const full = lines[li] ?? ''
    if (!full) return
    if (!deleting) {
      i += 1
      phVisible.value = full.slice(0, i)
      if (i >= full.length) {
        phHoldTimer = setTimeout(() => {
          deleting = true
          tick()
        }, 1800)
        return
      }
      phTimer = setTimeout(tick, 70)
      return
    }
    i -= 1
    phVisible.value = full.slice(0, Math.max(0, i))
    if (i <= 0) {
      deleting = false
      li = (li + 1) % lines.length
      phTimer = setTimeout(tick, 400)
      return
    }
    phTimer = setTimeout(tick, 32)
  }
  phTimer = setTimeout(tick, 80)
}

function autoGrow() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const h = Math.min(Math.max(el.scrollHeight, 88), 200)
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

function goProjects() {
  void router.push('/projects')
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
watch(placeholderLines, () => {
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
    <HomeParticleMeshBackground />
    <div
      class="home-shell__content relative z-[1] mx-auto flex w-full max-w-3xl flex-1 flex-col items-center justify-center px-4 py-10"
    >
      <h1 class="home-brand" data-testid="home-brand" aria-label="Approving">
        <span class="home-brand__text" data-testid="home-brand-text">{{ brandVisible }}</span>
        <span
          class="home-brand__cursor"
          :class="{
            'home-brand__cursor--blink': brandCursorBlink,
            'home-brand__cursor--gone': brandCursorGone,
          }"
          data-testid="home-brand-cursor"
          aria-hidden="true"
        />
      </h1>
      <p class="home-hint mt-[18px] text-center" data-testid="home-title">
        {{ t('pages.dashboard.title') }}
      </p>

      <div class="mt-[30px] w-full">
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
          class="mb-2 flex flex-wrap gap-2"
          data-testid="home-attach-chips"
        >
          <div v-for="(im, ii) in attachments" :key="ii" class="relative">
            <ChatImageThumb
              v-if="isImageAttachment(im)"
              mode="previewable"
              size="sm"
              thumb-class="rounded-none"
              :src="imgSrc(im)"
              :label="attachmentDisplayName(im, ii)"
              :alt="attachmentDisplayName(im, ii)"
              test-id="home-draft-image-thumb"
              @preview="openChatImagePreview(imgSrc(im), attachmentDisplayName(im, ii))"
            />
            <div
              v-else
              class="flex h-9 max-w-[160px] items-center gap-1.5 border border-line bg-elevated px-2.5"
              :title="attachmentDisplayName(im, ii)"
              data-testid="home-pending-file-chip"
            >
              <span class="shrink-0 text-[9px] font-semibold uppercase text-info">DOC</span>
              <span class="min-w-0 truncate text-[11px] text-txt2">{{
                attachmentDisplayName(im, ii)
              }}</span>
            </div>
            <button
              type="button"
              class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-none bg-err text-white"
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
          <div class="home-composer__field relative px-4 pb-3 pt-4">
            <label class="sr-only" for="home-composer-input">{{ placeholderPrimary }}</label>
            <textarea
              id="home-composer-input"
              ref="textareaRef"
              v-model="draft"
              class="home-composer__input w-full min-w-0 resize-none bg-transparent text-[15px] text-txt outline-none"
              :class="overflowScroll ? 'scroll-area max-h-[200px] overflow-y-auto' : 'overflow-y-hidden'"
              data-testid="home-composer-input"
              rows="3"
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
              class="home-composer__ph pointer-events-none absolute left-4 top-4 text-[15px] text-txt3"
              data-testid="home-composer-placeholder"
              aria-hidden="true"
            >
              {{ phVisible }}<span
                v-if="phCursor"
                class="home-composer__ph-cursor"
                data-testid="home-composer-ph-cursor"
              />
            </span>
          </div>
          <div class="home-composer__toolbar flex items-center gap-2 border-t px-3 py-2.5">
            <button
              type="button"
              class="home-composer__plus flex h-8 w-8 shrink-0 items-center justify-center border text-txt2 hover:text-txt disabled:opacity-40"
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
              class="home-composer__pipeline max-w-[12rem] min-w-0 cursor-pointer appearance-none border bg-transparent px-2.5 py-1.5 text-xs font-medium outline-none disabled:cursor-default disabled:text-txt3"
              data-testid="home-pipeline-select"
              :disabled="!pipelines.length || sending"
              :value="selectedId"
              @change="onPipelineChange"
            >
              <option v-if="!pipelines.length" value="">{{ t('pages.dashboard.noPipelineShort') }}</option>
              <option v-for="p in pipelines" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
            <div class="flex-1" />
            <button
              type="submit"
              class="home-composer__send flex h-8 w-8 shrink-0 items-center justify-center text-base disabled:opacity-[0.28]"
              data-testid="home-composer-send"
              :disabled="sending || !canSend"
              :aria-label="t('pages.dashboard.send')"
            >
              <Icon name="arrow-up" :size="16" />
            </button>
          </div>
        </form>
      </div>

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

      <div v-else-if="loading" class="mt-10 text-sm text-txt3" data-testid="home-pipelines-loading">
        {{ t('pages.board.loading') }}
      </div>

      <div v-else-if="!pipelines.length" class="mt-10 text-center" data-testid="home-pipelines-empty">
        <p class="text-sm text-txt3">{{ t('pages.dashboard.noPipelines') }}</p>
        <button
          type="button"
          class="mt-3 border border-line px-3 py-1.5 text-[13px] text-txt2 hover:bg-elevated"
          data-testid="home-go-projects"
          @click="goProjects"
        >
          {{ t('pages.dashboard.goProjects') }}
        </button>
      </div>

      <div
        v-else
        class="mt-10 flex w-full justify-center gap-3 overflow-x-auto pb-1"
        data-testid="home-pipeline-cards"
      >
        <button
          v-for="p in pipelines"
          :key="p.id"
          type="button"
          class="home-shell__card w-48 shrink-0 overflow-hidden border border-line p-0 text-left"
          :class="p.id === selected?.id ? 'home-shell__card--selected' : 'hover:border-line-strong'"
          :data-testid="`home-pipeline-card-${p.id}`"
          @click="selectPipeline(p.id)"
        >
          <div class="home-shell__card-top flex h-20 items-center justify-center">
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

    <ChatImagePreviewModal
      :open="!!imagePreview"
      :src="imagePreview?.src || ''"
      :label="imagePreview?.label || ''"
      test-id-prefix="home-image-preview"
      @close="closeChatImagePreview"
    />
  </div>
</template>

<style scoped>
/* g1 — clean shell aligned to page.html design tokens */
.home-shell {
  isolation: isolate;
}

/* g1.1 — monospace brand; solid color; letter-spacing matches design */
.home-brand {
  display: inline-flex;
  align-items: baseline;
  margin: 0;
  min-height: 1.1em;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  font-size: clamp(2.5rem, 8.5vw, 4.25rem);
  font-weight: 600;
  letter-spacing: 0.04em;
  line-height: 1.05;
  color: rgb(var(--c-txt));
}

.home-brand__text {
  white-space: pre;
}

.home-brand__cursor {
  display: inline-block;
  width: 0.08em;
  height: 0.92em;
  margin-left: 0.06em;
  vertical-align: -0.06em;
  flex-shrink: 0;
  background: rgb(var(--c-accent));
  opacity: 1;
  transition: opacity 0.2s ease;
}

.home-brand__cursor--blink {
  animation: home-brand-caret 0.85s steps(1) 3;
}

.home-brand__cursor--gone {
  opacity: 0;
}

.home-hint {
  font-size: 14px;
  font-weight: 500;
  letter-spacing: 0.01em;
  color: rgb(var(--c-txt2));
  opacity: 0;
  animation: home-hint-in 0.45s ease-out 0.15s forwards;
}

@keyframes home-hint-in {
  to {
    opacity: 1;
  }
}

/* g1.2 — right-angle composer (input + toolbar) */
.home-composer {
  border-color: rgb(var(--c-line));
  background: rgb(var(--c-surface));
}

:global(html.light) .home-composer {
  border-color: rgb(var(--c-line));
  background: rgb(var(--c-elevated));
}

.home-composer__toolbar {
  border-color: rgb(var(--c-line) / 0.55);
}

.home-composer__input {
  min-height: 88px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.home-composer__ph {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  word-break: break-word;
  line-height: 1.55;
}

.home-composer__ph-cursor {
  display: inline-block;
  width: 1.5px;
  height: 1.05em;
  margin-left: 1px;
  vertical-align: -2px;
  background: rgb(var(--c-accent));
  animation: home-ph-caret 1s steps(1) infinite;
}

.home-composer__plus {
  border-color: rgb(var(--c-line));
  background: transparent;
  transition: border-color 0.15s ease, color 0.15s ease, background-color 0.15s ease;
}

.home-composer__plus:hover:not(:disabled) {
  border-color: rgb(var(--c-line-strong));
  color: rgb(var(--c-txt));
  background: rgb(var(--c-accent) / 0.12);
}

.home-composer__pipeline {
  border-color: rgb(var(--c-line));
  color: rgb(var(--c-accent-2));
  height: 32px;
  background-image: linear-gradient(45deg, transparent 50%, rgb(var(--c-txt3)) 50%),
    linear-gradient(135deg, rgb(var(--c-txt3)) 50%, transparent 50%);
  background-position: calc(100% - 12px) 12px, calc(100% - 7px) 12px;
  background-size: 5px 5px, 5px 5px;
  background-repeat: no-repeat;
  padding-right: 26px;
}

.home-composer__pipeline:focus {
  border-color: rgb(var(--c-line-strong));
}

.home-composer__send {
  background: rgb(var(--c-txt));
  color: rgb(var(--c-base));
}

:global(html.light) .home-composer__send {
  background: rgb(var(--c-txt));
  color: #fff;
}

/* g1.2 — square cards; selected = accent inset border */
.home-shell__card {
  background: rgb(var(--c-surface));
  border-radius: 0;
  box-shadow: none;
}

:global(html.light) .home-shell__card {
  background: rgb(var(--c-elevated));
}

.home-shell__card--selected {
  border-color: rgb(var(--c-accent));
  box-shadow: inset 0 0 0 1px rgb(var(--c-accent));
}

.home-shell__card-top {
  background: rgb(var(--c-elevated));
  border-bottom: 1px solid rgb(var(--c-line) / 0.55);
}

:global(html.light) .home-shell__card-top {
  background: rgb(244 244 245);
}

@keyframes home-brand-caret {
  0%,
  49% {
    opacity: 1;
  }
  50%,
  100% {
    opacity: 0;
  }
}

@keyframes home-ph-caret {
  0%,
  49% {
    opacity: 1;
  }
  50%,
  100% {
    opacity: 0;
  }
}

/* g1.3 — prefers-reduced-motion */
@media (prefers-reduced-motion: reduce) {
  .home-hint {
    animation: none !important;
    opacity: 1;
  }

  .home-brand__cursor {
    opacity: 0 !important;
    animation: none !important;
  }

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

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../ui/Icon.vue'
import ChatImageThumb from '../ui/ChatImageThumb.vue'
import ChatImagePreviewModal from '../ui/ChatImagePreviewModal.vue'
import ClarifyDemoFrame from './ClarifyDemoFrame.vue'
import { renderMarkdown } from '@/lib/shared/markdown'
import { createStreamMarkdownPreview } from '@/lib/shared/streamMarkdownPreview'
import { createStreamTextReveal } from '@/lib/run/streamTextReveal'
import { mergePersistedAndLiveTurns, persistedCompletedLiveHuman } from '@/lib/inbox/mergeClarifyLiveTurns'
import { relTime } from '@/lib/shared/format'
import ThoughtSummaryStatus from './ThoughtSummaryStatus.vue'
import {
  demoGridColsClass,
  demoOptionsOf,
  selectedDemoOption,
  useSideBySide,
} from '@/lib/inbox/clarifyDemo'
import type {
  ClarifyTurn,
  ClarifyImage,
  ReactQuestion,
  ReactOption,
  ReactAnnotation,
  AcpEvent,
} from '@/lib/shared/types'
import AnnotationChip from './AnnotationChip.vue'
import {
  SITE_ATTACH_MAX_BYTES,
  SITE_ATTACH_MAX_MIB,
  attachmentDisplayName,
  fileAttachmentName,
  findOversizedAttachments,
  formatSelectRejectMessage,
  formatSendRejectMessage,
  isImageAttachment,
} from '@/lib/shared/attachments'
import { imgSrc } from '@/lib/shared/compositeText'
import { useChatImagePreview } from '@/lib/composables/useChatImagePreview'

type QueueItem = {
  id?: string
  text: string
  images: ClarifyImage[]
  annotations: ReactAnnotation[]
}

// active: whether the run is still in an interactive state (queued/running/
// waiting_human). When false (completed/failed/cancelled) the chat input is
// hidden — a finished run must not accept further replies.
// reviewMode: post-run ReAct review of a producer node's product (vs. the
// classic clarify dialogue). Relabels the finish action to "确认并流转";
// session UX (queue/stream/Cancel/refresh) is shared with clarify.
// annotateEnabled: show chip UI without forcing reviewMode finish semantics
// (inbox clarify binds annotations + review-mode affordances but only「发送澄清回复」).
// hideFinish: suppress finish / confirmFlow button (clarify send-only).
// confirmError: review force validation failure shown in the bottom status bar.
const props = withDefaults(
  defineProps<{
    runId: string
    nodeId: string
    iteration: number
    turns?: ClarifyTurn[] | null
    done: boolean
    active?: boolean
    reviewMode?: boolean
    annotateEnabled?: boolean
    hideFinish?: boolean
    /** Override primary send button label (e.g. 发送澄清回复). */
    sendLabel?: string
    /** Review confirm failure message (bottom status bar). */
    confirmError?: string | null
    /** Graph node type; Approve uses a first-speaker empty state. */
    nodeType?: string
  }>(),
  {
    active: true,
    iteration: 1,
    reviewMode: false,
    annotateEnabled: false,
    hideFinish: false,
    confirmError: null,
    nodeType: '',
    turns: () => [],
  },
)

const showAnnotationChips = computed(() => props.reviewMode || props.annotateEnabled)
const emit = defineEmits<{
  (e: 'send', text: string, images: ClarifyImage[], annotations: ReactAnnotation[]): void
  (e: 'finish'): void
  /** 轮级 Cancel (≠ confirm ≠ AbortRun). Review clears queue; clarify keeps it. */
  (e: 'cancel'): void
}>()

const { t: translate, locale } = useI18n()

const persistedTurns = computed(() => props.turns ?? [])

const inputPlaceholder = computed(() => {
  if (props.reviewMode) return translate('pages.clarify.reviewInputPlaceholder')
  if (props.nodeType === 'approve') return translate('pages.clarify.approveInputPlaceholder')
  return translate('pages.clarify.inputPlaceholder')
})

const draft = defineModel<string>('draft', { default: '' })
const attachments = defineModel<ClarifyImage[]>('attachments', { default: () => [] })
const annotations = defineModel<ReactAnnotation[]>('annotations', { default: () => [] })
const thinking = ref(false)
/** Review confirm mid-state: re-validating product (not Agent thinking). */
const validating = ref(false)
// SandboxChat-aligned pending-send queue (clarify + review). Items live ONLY in
// the bottom panel until turn_begin materializes transcript bubbles.
const queued = ref<QueueItem[]>([])
/** In-flight turn bubbles (human + streaming agent) before props.turns catch up. */
const liveTurns = ref<ClarifyTurn[]>([])
const showApproveEmptyHint = computed(
  () =>
    !props.reviewMode &&
    props.nodeType === 'approve' &&
    persistedTurns.value.length === 0 &&
    liveTurns.value.length === 0,
)
const liveAgentIdx = ref(-1)
/** Coalesced markdown HTML for the live streaming agent bubble. */
const liveStreamHtml = ref('')
const streamPreview = createStreamMarkdownPreview({ render: renderMarkdown })
const unsubStream = streamPreview.subscribe((html) => {
  liveStreamHtml.value = html
})
/** Coalesced thought text (rAF) — same cadence as message preview. */
const liveThoughtText = ref('')
const thoughtPreview = createStreamMarkdownPreview({ render: (s) => s })
const unsubThought = thoughtPreview.subscribe((text) => {
  liveThoughtText.value = text
})
/**
 * Smooth catch-up reveal (Demo) → then markdown/text coalesce.
 * Vitest uses sync so existing mid-stream assertions stay stable.
 */
const syncReveal = Boolean(import.meta.env.VITEST)
const messageReveal = createStreamTextReveal({
  sync: syncReveal,
  onReveal: (text) => {
    streamPreview.setText(text)
    // Vitest: flush markdown coalesce so mid-stream assertions see text without waiting rAF.
    if (syncReveal) streamPreview.flush()
  },
})
const thoughtReveal = createStreamTextReveal({
  sync: syncReveal,
  onReveal: (text) => {
    thoughtPreview.setText(text)
    if (syncReveal) thoughtPreview.flush()
  },
})
/**
 * Manual thought expand/collapse overrides (index → open).
 * Default: open while thought-only streaming; collapsed once message starts / done.
 */
const thoughtOpenOverride = ref<Record<number, boolean>>({})
const scroller = ref<HTMLElement>()
const fileInput = ref<HTMLInputElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const overflowScroll = ref(false)
const composing = ref(false)

/** Near-bottom threshold (px), aligned with PmLeaderChat / approved Demo. */
const STICK_THRESHOLD = 48
const stickToBottom = ref(true)
/** Unread real turns while off-bottom; never counts thinking placeholder. */
const unreadCount = ref(0)
const showUnreadFab = computed(() => unreadCount.value >= 1 && !stickToBottom.value)
const unreadFabLabel = computed(() =>
  translate('pages.clarify.unreadFab', { n: unreadCount.value }),
)

const sessionBusy = computed(
  () => thinking.value || queued.value.length > 0 || liveAgentIdx.value >= 0,
)

function isNearBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_THRESHOLD
}

function onScrollerScroll() {
  const el = scroller.value
  if (!el) return
  if (isNearBottom(el)) {
    stickToBottom.value = true
    unreadCount.value = 0
  } else {
    stickToBottom.value = false
  }
}

async function scrollBottom(force = false) {
  await nextTick()
  const el = scroller.value
  if (!el) return
  if (force || stickToBottom.value) {
    el.scrollTop = el.scrollHeight
    stickToBottom.value = true
    unreadCount.value = 0
  }
}

/**
 * Enter / remount / session-switch stick sequence (aligned with PmLeaderChat + Demo):
 * force pin immediately (scrollBottom already awaits nextTick), then re-pin after paint
 * so tall ReAct / high content blocks that lag one frame still land at the latest message.
 * Must not be used for incremental turn updates — those stay stick-gated.
 */
async function enterStickSequence() {
  stickToBottom.value = true
  unreadCount.value = 0
  await scrollBottom(true)
  requestAnimationFrame(() => {
    void scrollBottom(true)
  })
}

function onUnreadFabClick() {
  void scrollBottom(true)
}

const AUTO_GROW_MIN = 40
const AUTO_GROW_MAX = 128

function autoGrow() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const sh = el.scrollHeight
  const h = Math.min(Math.max(sh, AUTO_GROW_MIN), AUTO_GROW_MAX)
  el.style.height = `${h}px`
  overflowScroll.value = sh > AUTO_GROW_MAX
}

function onTextInput() {
  autoGrow()
}

// Optimistically show choice-card session replies until backend catches up.
// Authoritative send path uses queue + liveTurns (no pending human bubble).
const pending = ref<{ text: string; images: ClarifyImage[]; annotations: ReactAnnotation[] } | null>(null)

onBeforeUnmount(() => {
  unsubStream()
  unsubThought()
  messageReveal.reset()
  thoughtReveal.reset()
  streamPreview.reset()
  thoughtPreview.reset()
})

/** Legacy zh-CN prefix for messages persisted before i18n or in sessionStorage. */
const LEGACY_CHOICE_PREFIX = '我的选择:'
const choicePrefix = computed(() => translate('pages.clarify.choicePrefix'))

function textHasChoicePrefix(text: string): boolean {
  return text.startsWith(choicePrefix.value) || text.startsWith(LEGACY_CHOICE_PREFIX)
}

function stripChoicePrefix(text: string): string {
  if (text.startsWith(choicePrefix.value)) return text.slice(choicePrefix.value.length)
  if (text.startsWith(LEGACY_CHOICE_PREFIX)) return text.slice(LEGACY_CHOICE_PREFIX.length)
  return text
}

function questionKey(questions: ReactQuestion[]): string {
  return questions.map((q) => q.id).join('|')
}

function sessionStorageKey(questions: ReactQuestion[]): string {
  return `clarify.submitted.${props.runId}.${props.nodeId}.${props.iteration}.${questionKey(questions)}`
}

function readSessionChoice(questions: ReactQuestion[]): string | null {
  if (!questions.length) return null
  try {
    const raw = sessionStorage.getItem(sessionStorageKey(questions))
    if (!raw) return null
    const parsed = JSON.parse(raw) as { text?: string }
    return parsed.text && textHasChoicePrefix(parsed.text) ? parsed.text : null
  } catch {
    return null
  }
}

function writeSessionChoice(questions: ReactQuestion[], text: string) {
  if (!questions.length) return
  try {
    sessionStorage.setItem(sessionStorageKey(questions), JSON.stringify({ text }))
  } catch {}
}

function clearSessionChoice(questions: ReactQuestion[]) {
  if (!questions.length) return
  try {
    sessionStorage.removeItem(sessionStorageKey(questions))
  } catch {}
}

function isChoiceReply(text: string): boolean {
  return !!text && textHasChoicePrefix(text)
}

/** Index of the latest agent turn that raised structured questions. */
function latestQuestionTurnIndex(turnList: ClarifyTurn[]): number {
  for (let i = turnList.length - 1; i >= 0; i--) {
    const t = turnList[i]
    if (t.role === 'agent' && t.questions?.length) return i
  }
  return -1
}

function hasHumanReplyAfter(turnList: ClarifyTurn[], qIdx: number): boolean {
  for (let i = qIdx + 1; i < turnList.length; i++) {
    if (turnList[i].role === 'human' && isChoiceReply(turnList[i].text)) return true
  }
  return false
}

const latestQuestionIdx = computed(() => latestQuestionTurnIndex(persistedTurns.value))

const latestQuestions = computed<ReactQuestion[]>(() => {
  const idx = latestQuestionIdx.value
  if (idx < 0) return []
  return persistedTurns.value[idx].questions ?? []
})

const latestQuestionAnswered = computed(() => {
  const qs = latestQuestions.value
  if (!qs.length) return true
  if (hasHumanReplyAfter(persistedTurns.value, latestQuestionIdx.value)) return true
  return !!readSessionChoice(qs)
})

const displayTurns = computed<ClarifyTurn[]>(() => {
  let list = persistedTurns.value
  // Session UX: live in-flight bubbles until persisted transcript catches up.
  // Dedupe against persisted turns so mid-stream softRefresh/loadRun human does not double-render.
  if (liveTurns.value.length) {
    return mergePersistedAndLiveTurns(list, liveTurns.value)
  }
  // Choice-card sessionStorage echo only (not used for free-text send).
  if (pending.value) {
    list = [
      ...list,
      {
        role: 'human',
        text: pending.value.text,
        at: new Date().toISOString(),
        images: pending.value.images,
        annotations: pending.value.annotations,
      },
    ]
  } else {
    const qIdx = latestQuestionIdx.value
    const qs = latestQuestions.value
    if (qIdx >= 0 && qs.length && !hasHumanReplyAfter(persistedTurns.value, qIdx)) {
      const sessionText = readSessionChoice(qs)
      if (sessionText) {
        list = [...list, { role: 'human', text: sessionText, at: new Date().toISOString() }]
      }
    }
  }
  return list
})

/** Single-image lightbox for human history attachments (no gallery / Esc). */
const { preview: imagePreview, openChatImagePreview, closeChatImagePreview: closeImagePreview } =
  useChatImagePreview()

function imagePreviewLabel(images: ClarifyImage[], index: number): string {
  const named = images[index]?.name?.trim()
  if (named) return named
  if (images.length <= 1) return translate('common.chatImage.imageFallback')
  return translate('common.chatImage.imageFallbackN', { n: index + 1 })
}

const attachNotice = ref<string | null>(null)

function openImagePreview(images: ClarifyImage[], index: number) {
  const im = images[index]
  if (!im || !isImageAttachment(im)) return
  openChatImagePreview(imgSrc(im), imagePreviewLabel(images, index))
}

function turnsSemanticKey(turnList: ClarifyTurn[]): string {
  if (!turnList.length) return '0'
  const last = turnList[turnList.length - 1]
  return `${turnList.length}:${last.at}:${last.role}:${last.text}`
}

// Real turns streamed back from the run: drop optimistic choice echo + clear
// settled live bubbles. Scroll only when stuck to bottom.
watch(
  () => turnsSemanticKey(persistedTurns.value),
  (key, prevKey) => {
    if (prevKey !== undefined && key === prevKey) return
    const turnList = persistedTurns.value
    pending.value = null
    const liveHumanText = liveTurns.value[0]?.role === 'human' ? liveTurns.value[0].text || '' : ''
    const persistedCaughtUp = persistedCompletedLiveHuman(turnList, liveHumanText)
    // Clear live bubbles once persisted transcript includes them (not while streaming),
    // or when poll/refresh already persisted the completed human+agent pair.
    if (
      liveTurns.value.length &&
      (persistedCaughtUp || !liveTurns.value.some((t) => t.streaming))
    ) {
      liveTurns.value = []
      liveAgentIdx.value = -1
      messageReveal.reset()
      thoughtReveal.reset()
      streamPreview.reset()
      thoughtPreview.reset()
      if (queued.value.length === 0) thinking.value = false
    }
    const qIdx = latestQuestionTurnIndex(turnList)
    if (qIdx >= 0) {
      const qs = turnList[qIdx].questions ?? []
      if (qs.length && hasHumanReplyAfter(turnList, qIdx)) clearSessionChoice(qs)
    }
    void scrollBottom()
  },
)

// Exact unread from props.turns length deltas (ignores optimistic pending / thinking).
watch(
  () => persistedTurns.value.length,
  (len, prevLen) => {
    if (prevLen === undefined) return
    const delta = len - prevLen
    if (delta <= 0) {
      if (stickToBottom.value) unreadCount.value = 0
      return
    }
    if (stickToBottom.value) {
      unreadCount.value = 0
      void scrollBottom()
    } else {
      unreadCount.value += delta
    }
  },
)

// Session identity change: treat as enter — force pin + rAF second pin.
watch(
  () => `${props.runId}.${props.nodeId}.${props.iteration}`,
  () => {
    void enterStickSequence()
  },
)

// Thinking / validating placeholders follow only while stuck to bottom; never ++unread.
watch([thinking, validating], ([th, val], [prevTh, prevVal]) => {
  if ((th && !prevTh) || (val && !prevVal)) {
    if (stickToBottom.value) void scrollBottom()
  }
})
watch(
  () => props.done,
  (d) => {
    if (d) {
      thinking.value = false
      validating.value = false
    }
  },
)
// Parent surfaces review confirm failure → leave validating so the user can retry.
watch(
  () => props.confirmError,
  (err) => {
    if (err) validating.value = false
  },
)

function addFiles(files: FileList | null | undefined) {
  if (!files) return
  const rejected: string[] = []
  const list = Array.from(files)
  list.forEach((f, i) => {
    if (f.size > SITE_ATTACH_MAX_BYTES) {
      rejected.push(fileAttachmentName(f, i))
      return
    }
    const name = fileAttachmentName(f, i)
    const mimeType = f.type || 'application/octet-stream'
    const reader = new FileReader()
    reader.onload = () => {
      const res = String(reader.result || '')
      const comma = res.indexOf(',')
      attachments.value.push({
        data: comma >= 0 ? res.slice(comma + 1) : res,
        mimeType,
        name,
      })
    }
    reader.readAsDataURL(f)
  })
  if (rejected.length) {
    attachNotice.value = formatSelectRejectMessage(rejected, SITE_ATTACH_MAX_MIB)
  } else {
    attachNotice.value = null
  }
}
function onPickFiles(e: Event) {
  addFiles((e.target as HTMLInputElement).files)
  if (fileInput.value) fileInput.value.value = ''
}
function onPaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) {
    nextTick(autoGrow)
    return
  }
  const picked: File[] = []
  for (const it of Array.from(items)) {
    if (it.kind === 'file') {
      const f = it.getAsFile()
      if (f) picked.push(f)
    }
  }
  if (picked.length) {
    e.preventDefault()
    const dt = new DataTransfer()
    picked.forEach((f) => dt.items.add(f))
    addFiles(dt.files)
  }
  nextTick(autoGrow)
}

watch(draft, () => nextTick(autoGrow), { immediate: true })
onMounted(() => {
  nextTick(autoGrow)
  // Mount with historical turns: force enter stick (parent v-if remounts on leave/re-enter).
  void enterStickSequence()
})
function removeAttachment(i: number) {
  attachments.value.splice(i, 1)
}

function sendMessage(text: string, imgs: ClarifyImage[] = [], anns: ReactAnnotation[] = []) {
  const t = text.trim()
  if ((!t && imgs.length === 0 && anns.length === 0) || props.done || !props.active) return
  // Enqueue only — bubbles materialize on turn_begin (AgentChatTester / Demo).
  // Busy may still enqueue; never open a concurrent turn via optimistic pending.
  queued.value.push({ text: t, images: imgs, annotations: anns.slice() })
  thinking.value = true
  emit('send', t, imgs, anns)
  stickToBottom.value = true
  unreadCount.value = 0
  void scrollBottom(true)
}

function sendFromComposer() {
  const t = draft.value.trim()
  const imgs = attachments.value.slice()
  const anns = annotations.value.slice()
  if ((!t && imgs.length === 0 && anns.length === 0) || props.done || !props.active) return
  const over = findOversizedAttachments(imgs)
  if (over.length) {
    attachNotice.value = formatSendRejectMessage(
      over.map((im, i) => attachmentDisplayName(im, i)),
      SITE_ATTACH_MAX_MIB,
    )
    return
  }
  draft.value = ''
  attachments.value = []
  annotations.value = []
  attachNotice.value = null
  sendMessage(t, imgs, anns)
}

function onComposerKeydown(e: KeyboardEvent) {
  if (e.key !== 'Enter' || e.shiftKey) return
  // IME: composition Enter / keyCode 229 must not send (prevents double-fire).
  if (composing.value || e.isComposing || e.keyCode === 229) return
  e.preventDefault()
  send()
}

function removeAnnotation(i: number) {
  annotations.value.splice(i, 1)
}

// --- structured questions (ask_question) ---------------------------------
// Selected option ids per question, plus optional "其他" free text.
const sel = ref<Record<string, string[]>>({})
const other = ref<Record<string, string>>({})

// The interactive choice card is only shown for the latest unanswered agent
// question turn while the dialogue is live (not done / not mid-send).
const activeQuestions = computed<ReactQuestion[]>(() => {
  if (props.done || !props.active || thinking.value || pending.value) return []
  if (latestQuestionAnswered.value) return []
  return latestQuestions.value
})

function isActiveTurn(i: number): boolean {
  return activeQuestions.value.length > 0 && i === latestQuestionIdx.value
}
function isSelected(qidStr: string, oid: string): boolean {
  return (sel.value[qidStr] || []).includes(oid)
}
function toggle(q: ReactQuestion, oid: string) {
  const cur = sel.value[q.id] || []
  if (q.allowMultiple) {
    sel.value[q.id] = cur.includes(oid) ? cur.filter((x) => x !== oid) : [...cur, oid]
  } else {
    sel.value[q.id] = cur.includes(oid) ? [] : [oid]
  }
}
function answered(q: ReactQuestion): boolean {
  return (sel.value[q.id]?.length ?? 0) > 0 || !!other.value[q.id]?.trim()
}
const someAnswered = computed(() => activeQuestions.value.some(answered))

// --- recommended options -------------------------------------------------
// Auto-pick id for a question: the recommended option, or the first as a
// fallback (mirrors the backend auto_var / SelectRecommendedOption rule).
function autoPickId(q: ReactQuestion): string {
  const rec = q.options.find((o) => o.recommended)
  return rec ? rec.id : (q.options[0]?.id ?? '')
}
// Whether any active question carries an explicit recommendation — gates the
// "采用推荐" button and auto-recommend affordances.
const hasRecommended = computed(() => activeQuestions.value.some((q) => q.options.some((o) => o.recommended)))

// Fill sel with the recommended (or first) option for every active question.
function applyRecommended() {
  for (const q of activeQuestions.value) {
    const id = autoPickId(q)
    sel.value[q.id] = id ? [id] : []
  }
}
function submitRecommended() {
  if (!activeQuestions.value.length || thinking.value) return
  applyRecommended()
  submitChoices()
}

// --- card deck (one question per card, 上一题 / 下一张) ---------------------
const step = ref(0)
// Only a genuinely new question set (different question ids) resets the deck to
// the first card. Keyed by ids — not array identity — so that background run
// polling (which rebuilds `turns` into fresh objects each tick) does NOT snap
// the user back to the first question mid-answer.
watch(
  () => activeQuestions.value.map((q) => q.id).join('|'),
  (key) => {
    step.value = 0
    if (!key) return
    // Preselect the recommended option (if any) so the user only
    // confirms; never auto-pick the first when nothing was recommended.
    for (const q of activeQuestions.value) {
      const rec = q.options.find((o) => o.recommended)
      if (rec && !(sel.value[q.id]?.length ?? 0)) sel.value[q.id] = [rec.id]
    }
  },
)
const curQuestion = computed<ReactQuestion | null>(() => activeQuestions.value[step.value] || null)
const isFirstCard = computed(() => step.value <= 0)
const isLastCard = computed(() => step.value >= activeQuestions.value.length - 1)

function prevCard() {
  if (step.value > 0) step.value--
}
function nextCard() {
  if (step.value < activeQuestions.value.length - 1) step.value++
}
// Pick an option. Never auto-advances — the user moves on with 下一个.
function pick(q: ReactQuestion, oid: string) {
  toggle(q, oid)
}

function submitChoices() {
  const qs = activeQuestions.value
  if (!qs.length || !someAnswered.value || thinking.value) return
  // Skipped questions are omitted; only answered ones are summarized.
  const lines = qs.filter(answered).map((q) => {
    const picked = q.options.filter((o) => (sel.value[q.id] || []).includes(o.id)).map((o) => o.label)
    const extra = other.value[q.id]?.trim()
    if (extra) picked.push(extra)
    return `- ${q.prompt} → ${picked.join('、')}`
  })
  const text = choicePrefix.value + '\n' + lines.join('\n')
  writeSessionChoice(qs, text)
  sel.value = {}
  other.value = {}
  step.value = 0
  sendMessage(text)
}

// A submitted "我的选择:" human turn is rendered as structured cards instead of
// a raw markdown bullet list. Returns null for any other message.
interface ChoiceRow {
  q: string
  answers: string[]
}
function parseChoiceSummary(text: string): ChoiceRow[] | null {
  if (!text || !textHasChoicePrefix(text)) return null
  const bodyText = stripChoicePrefix(text)
  const rows: ChoiceRow[] = []
  for (const raw of bodyText.split('\n')) {
    const line = raw.trim()
    if (!line.startsWith('- ')) continue
    const body = line.slice(2)
    const idx = body.indexOf('→')
    if (idx === -1) continue
    const q = body.slice(0, idx).trim()
    const answers = body
      .slice(idx + 1)
      .split('、')
      .map((a) => a.trim())
      .filter(Boolean)
    if (q) rows.push({ q, answers })
  }
  return rows.length ? rows : null
}

function choiceRowsForAgentTurn(turnIndex: number): ChoiceRow[] | null {
  for (let j = turnIndex + 1; j < displayTurns.value.length; j++) {
    const turn = displayTurns.value[j]
    if (turn.role === 'human') {
      const parsed = parseChoiceSummary(turn.text)
      if (parsed) return parsed
    }
    if (turn.role === 'agent' && turn.questions?.length) break
  }
  return null
}

function selectedLabelsForQuestion(q: ReactQuestion, rows: ChoiceRow[] | null): string[] {
  if (!rows) return []
  return rows.find((r) => r.q === q.prompt)?.answers ?? []
}

function selectedDemoForInteractive(q: ReactQuestion): ReactOption | null {
  const demos = demoOptionsOf(q)
  if (!demos.length || useSideBySide(demos)) return null
  return q.options.find((o) => isSelected(q.id, o.id) && !!o.demoHtml?.trim()) ?? null
}

function send() {
  sendFromComposer()
}
// Clarify: force Agent wrap-up (disabled while thinking).
// Review: accept store snapshot only when ready (not thinking / queue empty).
function finishEarly() {
  if (props.done || !props.active) return
  if (props.reviewMode) {
    if (validating.value || confirmDisabled.value) return
    validating.value = true
    emit('finish')
    void scrollBottom()
    return
  }
  if (thinking.value) return
  thinking.value = true
  emit('finish')
  void scrollBottom()
}

const confirmDisabled = computed(() => {
  // Force finish / 确认并流转: only when session idle (no in-flight / queue).
  return validating.value || thinking.value || queued.value.length > 0 || liveAgentIdx.value >= 0
})

function cancelReview() {
  if (props.done || !sessionBusy.value) return
  // Review: clear local queue (matches CancelReviewSession clear-queue).
  // Clarify: keep local queue; authoritative queue_state will reconcile (Demo).
  if (props.reviewMode) {
    queued.value = []
  }
  if (liveAgentIdx.value >= 0 && liveTurns.value[liveAgentIdx.value]) {
    liveTurns.value[liveAgentIdx.value].streaming = false
    liveTurns.value[liveAgentIdx.value].interrupted = true
    if (!liveTurns.value[liveAgentIdx.value].text) {
      liveTurns.value[liveAgentIdx.value].text = '(已中断)'
    }
  }
  liveAgentIdx.value = -1
  messageReveal.reset()
  thoughtReveal.reset()
  streamPreview.reset()
  thoughtPreview.reset()
  thinking.value = queued.value.length > 0
  emit('cancel')
  void scrollBottom()
}

/**
 * Parent calls this when reactReply(force=false) fails after optimistic enqueue,
 * so the pending-send panel and FR4 confirm gate do not stick forever.
 */
function discardLastQueued() {
  if (queued.value.length === 0) return
  queued.value.pop()
  if (queued.value.length === 0 && liveAgentIdx.value < 0) {
    thinking.value = false
  }
}

/**
 * Authoritative idle (waiting=0 ∧ !busy ∧ no activeItem): force-clear local
 * sticky busy — ghost queued, unfinished live slot (incl. body + streaming===false),
 * and thinking. Used by queue_state/poll idle (not turn_done — that must keep
 * real server-id waiters).
 */
function forceAuthoritativeIdle() {
  if (queued.value.length) queued.value = []
  if (liveAgentIdx.value >= 0) {
    const agent = liveTurns.value[liveAgentIdx.value]
    const empty = !!agent && !agent.text && !agent.thought
    if (empty) {
      liveTurns.value = []
      streamPreview.reset()
      thoughtPreview.reset()
    } else if (agent) {
      if (agent.streaming) {
        agent.streaming = false
        streamPreview.flush()
        thoughtPreview.flush()
      }
    }
    liveAgentIdx.value = -1
  }
  thinking.value = false
}

/**
 * After turn_done/error: drop only ghost optimistic rows (no server id).
 * Keep authoritative waiters so multi-turn queue does not briefly unlock
 * confirm before the next queue_state. Empty after filter ⇒ synthesized idle.
 */
function settleAfterTurnEnd() {
  if (queued.value.some((q) => !q.id)) {
    queued.value = queued.value.filter((q) => !!q.id)
  }
  thinking.value = queued.value.length > 0 || liveAgentIdx.value >= 0
}

/**
 * Reconcile local pending-send panel with platform-authoritative queue_state.
 * waiting===0 clears ghost rows after remote Cancel; items[] rebuilds/trims
 * while allowing at most one local-ahead item when no live turn (HTTP ack /
 * turn_begin in flight). busy+activeItem restores live bubble after refresh.
 */
function applyQueueState(
  waiting: number,
  items: { id?: string; text?: string }[] | null,
  busy?: boolean,
  activeItem?: {
    id?: string
    text?: string
    images?: ClarifyImage[]
    annotations?: ReactAnnotation[]
  } | null,
) {
  const authoritativeIdle = waiting === 0 && !busy && !activeItem
  if (authoritativeIdle) {
    forceAuthoritativeIdle()
    return
  }
  if (waiting === 0 && !busy) {
    if (queued.value.length) queued.value = []
    if (liveAgentIdx.value < 0) thinking.value = false
  } else if (items) {
    const rebuilt: QueueItem[] = items.map((it) => {
      const text = it.text ?? ''
      const id = typeof it.id === 'string' && it.id ? it.id : undefined
      // Prefer server id match; text fallback for optimistic rows not yet reconciled.
      const local = id
        ? queued.value.find((q) => q.id === id) ?? queued.value.find((q) => !q.id && q.text === text)
        : queued.value.find((q) => q.text === text)
      return {
        id: id ?? local?.id,
        text,
        images: local?.images ?? [],
        annotations: local?.annotations ?? [],
      }
    })
    const maxLocal = liveAgentIdx.value >= 0 || busy ? rebuilt.length : rebuilt.length + 1
    if (queued.value.length > maxLocal) {
      const optimistic = queued.value.slice(rebuilt.length).slice(0, Math.max(0, maxLocal - rebuilt.length))
      queued.value = [...rebuilt, ...optimistic]
    } else if (queued.value.length < rebuilt.length) {
      queued.value = rebuilt
    } else {
      const optimistic = queued.value.slice(rebuilt.length)
      queued.value = [...rebuilt, ...optimistic]
    }
  }
  // Refresh resume: recreate streaming agent bubble from activeItem when busy.
  // Skip when persisted turns already completed this human — otherwise poll
  // resume duplicates the bubble and leaves a stuck「思考中…」placeholder.
  if (
    busy &&
    activeItem &&
    liveAgentIdx.value < 0 &&
    !persistedCompletedLiveHuman(persistedTurns.value, activeItem.text ?? '')
  ) {
    const text = activeItem.text ?? ''
    liveTurns.value = [
      {
        role: 'human',
        text,
        at: new Date().toISOString(),
        images: activeItem.images,
        annotations: activeItem.annotations,
      },
      {
        role: 'agent',
        text: '',
        thought: '',
        at: new Date().toISOString(),
        streaming: true,
      },
    ]
    liveAgentIdx.value = 1
    thoughtOpenOverride.value = {}
    messageReveal.reset()
    thoughtReveal.reset()
    streamPreview.reset()
    thoughtPreview.reset()
  }
  // Authority idle / !busy: tear down live slot — empty placeholder or finished
  // body with streaming===false must not keep thinking stuck true.
  if (!busy && liveAgentIdx.value >= 0) {
    const agent = liveTurns.value[liveAgentIdx.value]
    const empty = !!agent && !agent.text && !agent.thought
    if (empty) {
      liveTurns.value = []
      liveAgentIdx.value = -1
      streamPreview.reset()
      thoughtPreview.reset()
    } else {
      if (agent?.streaming) {
        agent.streaming = false
        streamPreview.flush()
        thoughtPreview.flush()
      }
      liveAgentIdx.value = -1
    }
  }
  thinking.value = liveAgentIdx.value >= 0 || queued.value.length > 0 || !!busy
}

/** Parent feeds review/clarify WS frames here (shared session protocol). */
function applyReviewFrame(frame: {
  event?: string
  nodeId?: string
  item?: {
    id?: string
    text?: string
    images?: ClarifyImage[]
    annotations?: ReactAnnotation[]
  }
  message?: string
  interrupted?: boolean
  waiting?: number
  items?: { id?: string; text?: string }[]
  busy?: boolean
  activeItem?: {
    id?: string
    text?: string
    images?: ClarifyImage[]
    annotations?: ReactAnnotation[]
  }
}) {
  // Defense: ignore frames for other producer sessions on the same run.
  if (frame.nodeId && frame.nodeId !== props.nodeId) return
  switch (frame.event) {
    case 'turn_begin': {
      // Pump order: queue_state(remaining, no active) → turn_begin(item=active).
      // frame.item is server-authoritative; match-remove by id first, text fallback —
      // never blind-shift (that would bind live human to the next waiter after trim).
      const auth = frame.item
      const id = typeof auth?.id === 'string' && auth.id ? auth.id : undefined
      const text = auth?.text ?? ''
      let matchIdx = -1
      if (id) {
        matchIdx = queued.value.findIndex((q) => q.id === id)
      } else if (text) {
        // No server id: text match for optimistic rows only.
        matchIdx = queued.value.findIndex((q) => q.text === text)
      }
      // If auth carried an id but it is already absent (queue_state trimmed it),
      // do NOT fall back to text — that would steal a different same-text waiter.
      const local = matchIdx >= 0 ? queued.value.splice(matchIdx, 1)[0] : undefined
      const images =
        auth?.images && auth.images.length > 0 ? auth.images : local?.images
      const annotations =
        auth?.annotations && auth.annotations.length > 0
          ? auth.annotations
          : local?.annotations
      liveTurns.value.push({
        role: 'human',
        text: text || local?.text || '',
        at: new Date().toISOString(),
        images,
        annotations,
      })
      liveAgentIdx.value =
        liveTurns.value.push({
          role: 'agent',
          text: '',
          thought: '',
          at: new Date().toISOString(),
          streaming: true,
        }) - 1
      thoughtOpenOverride.value = {}
      messageReveal.reset()
      thoughtReveal.reset()
      streamPreview.reset()
      thoughtPreview.reset()
      thinking.value = true
      break
    }
    case 'turn_done': {
      if (liveAgentIdx.value >= 0 && liveTurns.value[liveAgentIdx.value]) {
        const agent = liveTurns.value[liveAgentIdx.value]
        agent.streaming = false
        agent.at = new Date().toISOString()
        if (frame.interrupted) agent.interrupted = true
        // Reveal flush before markdown flush (plan g1.2).
        messageReveal.flush()
        thoughtReveal.flush()
        streamPreview.flush()
        thoughtPreview.flush()
      }
      liveAgentIdx.value = -1
      // Pump may omit idle queue_state; clear ghost optimistic rows only.
      // Real id waiters stay busy until authoritative idle (review v1).
      settleAfterTurnEnd()
      break
    }
    case 'error': {
      if (liveAgentIdx.value >= 0 && liveTurns.value[liveAgentIdx.value]) {
        liveTurns.value[liveAgentIdx.value].streaming = false
        liveTurns.value[liveAgentIdx.value].text =
          liveTurns.value[liveAgentIdx.value].text || frame.message || 'error'
        liveTurns.value[liveAgentIdx.value].at = new Date().toISOString()
        if (frame.interrupted) liveTurns.value[liveAgentIdx.value].interrupted = true
        messageReveal.flush()
        thoughtReveal.flush()
        streamPreview.flush()
        thoughtPreview.flush()
      }
      liveAgentIdx.value = -1
      // Same as turn_done: ghosts only; keep real remaining waiters.
      settleAfterTurnEnd()
      break
    }
    case 'queue_state':
      applyQueueState(
        typeof frame.waiting === 'number' ? frame.waiting : 0,
        Array.isArray(frame.items) ? frame.items : null,
        frame.busy,
        frame.activeItem ?? null,
      )
      break
  }
  void scrollBottom()
}

/**
 * Consume publishAcp events for the streaming agent bubble (dialogue surface).
 * Returns false when mounted but streaming slot is not ready — host must buffer
 * (hard-load / remount race; never silent-noop as applied).
 */
function applyAcpEvents(events: AcpEvent[] | undefined, nodeId?: string): boolean {
  if (!events?.length) return true
  // Wrong node: not a readiness failure — do not buffer for another surface.
  if (nodeId && nodeId !== props.nodeId) return true
  // Slot not rebuilt yet (queue_state pending) — buffer at host.
  if (liveAgentIdx.value < 0) return false
  const agent = liveTurns.value[liveAgentIdx.value]
  if (!agent) return false
  let msg = agent.text
  let thought = agent.thought || ''
  for (const ev of events) {
    // Ignore tool_call / plan for UI (Demo: no tool chips); placeholder stays via streaming.
    if (ev.kind === 'message' && ev.text) msg = ev.text
    if (ev.kind === 'thought' && ev.text) thought = ev.text
  }
  // Keep thought / message on separate rails — never msg||thought overwrite.
  agent.thought = thought
  agent.text = msg
  // Authority → reveal → markdown/text coalesce (not absolute snapshot → DOM).
  messageReveal.setTarget(msg)
  thoughtReveal.setTarget(thought)
  // Stick-gated only — never force-drag while user scrolled up.
  void scrollBottom()
  return true
}

/** Live agent bubble has visible message body (text or coalesced markdown). */
function agentHasMessage(t: ClarifyTurn): boolean {
  return !!(t.text || (t.streaming && liveStreamHtml.value))
}

/** Display thought (revealed while this live agent is streaming). */
function agentThoughtDisplay(t: ClarifyTurn, idx: number): string {
  if (t.streaming && idx === liveAgentIdx.value) {
    return liveThoughtText.value
  }
  return t.thought || ''
}

/** Default open while thought-only; collapse once message exists (Demo). */
function isThoughtOpen(idx: number, t: ClarifyTurn): boolean {
  if (thoughtOpenOverride.value[idx] !== undefined) {
    return thoughtOpenOverride.value[idx]!
  }
  return !!(t.streaming && t.thought && !agentHasMessage(t))
}

function onThoughtToggle(idx: number, e: Event) {
  const el = e.target as HTMLDetailsElement
  thoughtOpenOverride.value = { ...thoughtOpenOverride.value, [idx]: el.open }
}

/** Normal completion footnote (not interrupted / error). */
function showTurnCompleted(t: ClarifyTurn): boolean {
  return (
    t.role === 'agent' &&
    !t.streaming &&
    !t.interrupted &&
    !!(t.text || t.thought)
  )
}

defineExpose({
  applyReviewFrame,
  applyAcpEvents,
  applyQueueState,
  cancelReview,
  discardLastQueued,
  /** Host narrow-update gate: skip loadRun/softRefresh while true. */
  isSessionBusy: () => sessionBusy.value,
})
</script>

<template>
  <div class="flex h-full flex-col" data-review-composer>
    <div class="relative flex min-h-0 flex-1 flex-col">
    <div
      ref="scroller"
      class="scroll-area flex-1 space-y-3 overflow-y-auto p-4 pb-14"
      data-testid="clarify-scroller"
      @scroll.passive="onScrollerScroll"
    >
      <div class="flex items-center gap-2 text-[11px] text-txt3">
        <Icon name="chat" :size="13" />
        {{ translate('pages.clarify.header', { n: displayTurns.length }) }}
      </div>
      <p
        v-if="showApproveEmptyHint"
        class="text-[12px] leading-relaxed text-txt2"
        data-testid="clarify-approve-empty-hint"
      >
        {{ translate('pages.clarify.approveEmptyHint') }}
      </p>
      <div v-for="(t, i) in displayTurns" :key="i" class="flex gap-2.5" :class="t.role === 'human' ? 'flex-row-reverse' : ''">
        <div
          class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold"
          :class="t.role === 'agent' ? 'bg-n-clarify/15 text-n-clarify' : 'bg-accent-dim text-accent-2'"
        >
          <Icon v-if="t.role === 'agent'" name="robot" :size="15" />
          <span v-else>{{ translate('pages.clarify.me') }}</span>
        </div>
        <div class="min-w-0 max-w-[80%]">
          <div v-if="t.images && t.images.length" class="mb-1.5 flex flex-wrap gap-1.5" :class="t.role === 'human' ? 'justify-end' : ''">
            <!-- human history: images → lightbox; non-images → filename chip -->
            <template v-if="t.role === 'human'">
              <template v-for="(im, ii) in t.images" :key="ii">
                <ChatImageThumb
                  v-if="isImageAttachment(im)"
                  mode="previewable"
                  size="md"
                  thumb-class="rounded-md"
                  :src="imgSrc(im)"
                  :label="imagePreviewLabel(t.images, ii)"
                  test-id="clarify-history-image-thumb"
                  @preview="openImagePreview(t.images!, ii)"
                />
                <div
                  v-else
                  class="flex max-w-[200px] items-center gap-2 border border-line bg-elevated px-2 py-1.5"
                  data-testid="clarify-history-file-chip"
                  :title="imagePreviewLabel(t.images, ii)"
                >
                  <span class="shrink-0 text-[10px] font-medium uppercase tracking-wide text-info">DOC</span>
                  <span class="min-w-0 truncate text-[12px] text-txt">{{ imagePreviewLabel(t.images, ii) }}</span>
                </div>
              </template>
            </template>
            <!-- agent history: locked thumbs / filename chips (FR-f7) -->
            <template v-else>
              <template v-for="(im, ii) in t.images" :key="ii">
                <ChatImageThumb
                  v-if="isImageAttachment(im)"
                  mode="locked"
                  size="md"
                  thumb-class="rounded-md"
                  :src="imgSrc(im)"
                  :label="imagePreviewLabel(t.images, ii)"
                  test-id="clarify-agent-image-thumb"
                />
                <div
                  v-else
                  class="flex max-w-[200px] items-center gap-2 border border-line bg-elevated px-2 py-1.5"
                  data-testid="clarify-agent-file-chip"
                >
                  <span class="shrink-0 text-[10px] font-medium uppercase tracking-wide text-info">DOC</span>
                  <span class="min-w-0 truncate text-[12px] text-txt">{{ imagePreviewLabel(t.images, ii) }}</span>
                </div>
              </template>
            </template>
          </div>
          <!-- annotation chips attached to this human review turn -->
          <div v-if="t.role === 'human' && t.annotations && t.annotations.length" class="mb-1.5 flex flex-wrap gap-1.5 justify-end">
            <AnnotationChip
              v-for="(a, ai) in t.annotations"
              :key="ai"
              :ann="a"
            />
          </div>
          <!-- submitted choice summary → structured cards -->
          <div
            v-if="t.role === 'human' && parseChoiceSummary(t.text)"
            class="rounded-lg border border-accent/30 bg-accent-dim/60 px-3 py-2"
          >
            <div class="mb-2 flex items-center gap-1.5 text-[11px] font-medium text-txt2">
              <Icon name="check" :size="12" class="text-accent" /> {{ translate('pages.clarify.myChoice') }}
            </div>
            <div class="space-y-1.5">
              <div
                v-for="(row, ri) in parseChoiceSummary(t.text)!"
                :key="ri"
                class="rounded-md border border-line bg-surface/70 px-2.5 py-1.5"
              >
                <div class="mb-1 text-[11px] leading-snug text-txt3">{{ row.q }}</div>
                <div class="flex flex-wrap gap-1">
                  <span
                    v-for="(a, ai) in row.answers"
                    :key="ai"
                    class="rounded border border-accent/30 bg-accent/10 px-1.5 py-0.5 text-[12px] text-txt"
                  >{{ a }}</span>
                </div>
              </div>
            </div>
          </div>
          <!-- Agent busy / thought / message (Demo: four-phase + restrained done) -->
          <template v-else-if="t.role === 'agent'">
            <!-- Waiting first token: dots + 思考中… inside bubble -->
            <div
              v-if="t.streaming && !t.thought && !agentHasMessage(t)"
              class="inline-flex items-center gap-2.5 rounded-lg border border-line bg-elevated px-3 py-2 text-[13px] text-txt3"
              data-testid="clarify-busy-placeholder"
            >
              <span class="typing-dots" aria-hidden="true"><i /><i /><i /></span>
              <span>{{ translate('pages.clarify.thinkingBusy') }}</span>
            </div>
            <!-- Status: 思考中… (has thought, no message yet) -->
            <div
              v-else-if="t.streaming && t.thought && !agentHasMessage(t)"
              class="mb-1.5 text-[12px] font-normal text-txt3"
              data-testid="clarify-busy-status"
            >
              {{ translate('pages.clarify.thinkingBusy') }}
            </div>
            <!-- Status: 输出中 (shimmer) while streaming message -->
            <div
              v-else-if="t.streaming && agentHasMessage(t)"
              class="clarify-outputting mb-1.5 text-[12px] font-normal"
              data-testid="clarify-busy-status"
            >
              {{ translate('pages.clarify.outputting') }}
            </div>
            <!-- Thought: open while thought-only; default collapsed once message starts -->
            <details
              v-if="t.thought"
              class="mb-2 w-full rounded-md border border-line bg-base/60 text-[11.5px] text-txt3"
              data-testid="clarify-thought"
              :open="isThoughtOpen(i, t)"
              @toggle="onThoughtToggle(i, $event)"
            >
              <summary
                class="flex cursor-pointer select-none items-center gap-1.5 px-2.5 py-1.5 text-txt3 hover:text-txt2"
                data-testid="clarify-thought-summary"
              >
                <ThoughtSummaryStatus
                  :busy="!!t.streaming"
                  :completed="showTurnCompleted(t)"
                  :interrupted="!!t.interrupted"
                />
              </summary>
              <div class="whitespace-pre-wrap break-words border-t border-dashed border-line px-2.5 pb-2 pt-1.5 font-mono leading-5 [overflow-wrap:anywhere]">{{ agentThoughtDisplay(t, i) }}</div>
            </details>
            <!-- Message body + streaming caret -->
            <div
              v-if="agentHasMessage(t)"
              class="md rounded-lg border border-line bg-elevated px-3 py-2 text-[13px] leading-relaxed text-txt"
              data-testid="clarify-agent-message"
            >
              <span
                v-html="t.streaming ? liveStreamHtml : renderMarkdown(t.text)"
              /><span
                v-if="t.streaming"
                class="clarify-stream-caret"
                data-testid="clarify-stream-caret"
                aria-hidden="true"
              />
            </div>
            <!-- Restrained completion footnote (Demo); never for interrupted/error -->
            <div
              v-if="showTurnCompleted(t)"
              class="mt-1.5 flex items-center justify-between gap-2 text-[11px] text-txt3"
              data-testid="clarify-turn-completed"
            >
              <span class="text-txt2">{{ translate('pages.clarify.turnCompleted') }}</span>
              <span>{{ relTime(t.at) }}</span>
            </div>
          </template>
          <!-- Human free-text bubble (agent branch handled above; role narrowed to human) -->
          <div
            v-else-if="t.text"
            class="md rounded-lg border border-accent/30 bg-accent-dim/60 px-3 py-2 text-[13px] leading-relaxed text-txt"
            v-html="renderMarkdown(t.text)"
          />

          <!-- Structured choice questions (ask_question). The latest agent turn
               shows an interactive card deck (one question per card); earlier
               turns render read-only for context. -->
          <template v-if="t.role === 'agent' && t.questions && t.questions.length">
            <!-- interactive card deck -->
            <div v-if="isActiveTurn(i) && curQuestion" class="mt-2">
              <div>
                <div class="relative rounded-xl border border-n-clarify/25 bg-n-clarify/5 p-3">
                  <!-- progress -->
                  <div v-if="activeQuestions.length > 1" class="mb-2.5 flex items-center justify-between">
                    <div class="flex items-center gap-1">
                      <span
                        v-for="(q, qi) in activeQuestions"
                        :key="qi"
                        class="h-1.5 rounded-full transition-all"
                        :class="qi === step ? 'w-4 bg-n-clarify' : answered(q) ? 'w-1.5 bg-n-clarify/60' : 'w-1.5 bg-line-strong'"
                      />
                    </div>
                    <span class="text-[11px] text-txt3">{{ step + 1 }} / {{ activeQuestions.length }}</span>
                  </div>

                  <Transition name="deck" mode="out-in">
                    <div :key="step">
                      <div class="mb-2 flex items-center gap-1.5 text-[13px] font-medium text-txt">
                        <Icon name="chat" :size="13" class="shrink-0 text-n-clarify" />
                        <span>{{ curQuestion.prompt }}</span>
                        <span class="ml-auto shrink-0 rounded border border-line px-1.5 py-0.5 text-[10px] font-normal text-txt3">{{ curQuestion.allowMultiple ? translate('pages.clarify.multiple') : translate('pages.clarify.single') }}</span>
                      </div>
                      <div class="space-y-1.5">
                        <button
                          v-for="o in curQuestion.options"
                          :key="o.id"
                          type="button"
                          class="flex w-full items-center gap-2 rounded-lg border px-2.5 py-2 text-left text-[12px] transition-colors"
                          :class="isSelected(curQuestion.id, o.id) ? 'border-accent bg-accent-dim/60 text-txt' : 'border-line bg-surface text-txt2 hover:border-line-strong'"
                          @click="pick(curQuestion, o.id)"
                        >
                          <span
                            class="flex h-4 w-4 shrink-0 items-center justify-center border"
                            :class="[
                              curQuestion.allowMultiple ? 'rounded' : 'rounded-full',
                              isSelected(curQuestion.id, o.id) ? 'border-accent bg-accent text-white' : 'border-line-strong',
                            ]"
                          >
                            <Icon v-if="isSelected(curQuestion.id, o.id)" name="check" :size="10" />
                          </span>
                          <span>{{ o.label }}</span>
                          <span
                            v-if="o.recommended"
                            class="ml-auto shrink-0 rounded-full bg-accent/15 px-1.5 py-0.5 text-[10px] font-medium text-accent-2"
                          >{{ translate('pages.clarify.recommended') }}</span>
                        </button>
                      </div>
                      <input
                        v-model="other[curQuestion.id]"
                        type="text"
                        class="input mt-1.5 h-8 w-full text-[12px]"
                        :placeholder="translate('pages.clarify.otherPlaceholder')"
                      />

                      <!-- Demo previews (options with demoHtml) -->
                      <div v-if="demoOptionsOf(curQuestion).length" class="mt-2.5">
                        <div
                          v-if="useSideBySide(demoOptionsOf(curQuestion))"
                          class="grid gap-2.5"
                          :class="demoGridColsClass(demoOptionsOf(curQuestion).length)"
                        >
                          <ClarifyDemoFrame
                            v-for="o in demoOptionsOf(curQuestion)"
                            :key="o.id"
                            :label="o.label"
                            :html="o.demoHtml!"
                            :highlighted="isSelected(curQuestion.id, o.id)"
                          />
                        </div>
                        <ClarifyDemoFrame
                          v-else-if="selectedDemoForInteractive(curQuestion)"
                          :label="selectedDemoForInteractive(curQuestion)!.label"
                          :html="selectedDemoForInteractive(curQuestion)!.demoHtml!"
                          highlighted
                        />
                      </div>
                    </div>
                  </Transition>

                  <!-- actions -->
                  <div class="mt-3 flex items-center justify-between">
                    <button
                      v-if="!isFirstCard"
                      class="inline-flex items-center gap-0.5 rounded-md px-2 py-1 text-[12px] text-txt2 hover:text-txt"
                      @click="prevCard"
                    >
                      <Icon name="arrow-left" :size="13" /> {{ translate('pages.clarify.prev') }}
                    </button>
                    <span v-else />

                    <button
                      v-if="!isLastCard"
                      class="inline-flex items-center gap-0.5 rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white hover:bg-accent-2"
                      @click="nextCard"
                    >
                      {{ translate('pages.clarify.next') }} <Icon name="chevron-right" :size="13" />
                    </button>
                    <div v-else class="flex items-center gap-2">
                      <button
                        v-if="hasRecommended"
                        class="inline-flex items-center gap-1 rounded-md border border-accent/40 bg-accent/10 px-2.5 py-1.5 text-[12px] font-medium text-accent-2 hover:bg-accent/20 disabled:opacity-50"
                        :disabled="thinking"
                        :title="translate('pages.clarify.applyRecommendedTitle')"
                        @click="submitRecommended"
                      >
                        <Icon name="check" :size="12" /> {{ translate('pages.clarify.applyRecommended') }}
                      </button>
                      <button
                        class="inline-flex items-center gap-1 rounded-md bg-accent px-3 py-1.5 text-[12px] font-medium text-white hover:bg-accent-2 disabled:opacity-50"
                        :disabled="!someAnswered || thinking"
                        @click="submitChoices"
                      >
                        <Icon name="send" :size="12" /> {{ translate('pages.clarify.submitChoices') }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- read-only history: show demo previews when demoHtml present -->
            <div v-else class="mt-2 rounded-lg border border-n-clarify/20 bg-n-clarify/5 px-3 py-2">
              <div class="mb-1.5 flex items-center gap-1.5 text-[11px] font-medium text-txt3">
                <Icon name="chat" :size="12" class="text-n-clarify" />
                {{ translate('pages.clarify.questionsThisRound', { n: t.questions.length }) }}
                <span class="ml-1 inline-flex items-center gap-1 rounded border border-line px-1.5 py-0.5 text-[10px] font-normal text-txt3">
                  {{ translate('pages.clarify.readonly') }}
                </span>
              </div>
              <div
                v-for="(q, qi) in t.questions"
                :key="qi"
                class="mb-3 last:mb-0"
              >
                <div class="mb-1.5 text-[12px] leading-snug text-txt2">
                  <span class="text-txt3">{{ qi + 1 }}.</span> {{ q.prompt }}
                  <span
                    v-for="o in q.options.filter((op) => op.recommended)"
                    :key="o.id"
                    class="ml-1 inline-flex rounded-full bg-accent/15 px-1.5 py-0.5 text-[10px] font-medium text-accent-2"
                  >{{ translate('pages.clarify.recommendedLabel', { label: o.label }) }}</span>
                </div>
                <div v-if="demoOptionsOf(q).length" class="mt-1.5">
                  <div
                    v-if="useSideBySide(demoOptionsOf(q))"
                    class="grid gap-2.5"
                    :class="demoGridColsClass(demoOptionsOf(q).length)"
                  >
                    <ClarifyDemoFrame
                      v-for="o in demoOptionsOf(q)"
                      :key="o.id"
                      :label="o.label"
                      :html="o.demoHtml!"
                      :highlighted="selectedLabelsForQuestion(q, choiceRowsForAgentTurn(i)).includes(o.label)"
                      :selected="selectedLabelsForQuestion(q, choiceRowsForAgentTurn(i)).includes(o.label)"
                    />
                  </div>
                  <ClarifyDemoFrame
                    v-else-if="selectedDemoOption(q, selectedLabelsForQuestion(q, choiceRowsForAgentTurn(i)))"
                    :label="selectedDemoOption(q, selectedLabelsForQuestion(q, choiceRowsForAgentTurn(i)))!.label"
                    :html="selectedDemoOption(q, selectedLabelsForQuestion(q, choiceRowsForAgentTurn(i)))!.demoHtml!"
                    highlighted
                    selected
                  />
                </div>
              </div>
            </div>
          </template>

          <div
            v-if="t.interrupted"
            class="mt-1 inline-flex items-center gap-1 rounded border border-warn/40 bg-warn/10 px-1.5 py-0.5 text-[10px] text-warn"
            data-testid="clarify-interrupted"
          >
            interrupted
          </div>
          <!-- Keep footer time; hide bottom time when completion footnote is shown (keep_footer_hide_bottom) -->
          <div
            v-if="!showTurnCompleted(t)"
            class="mt-1 text-[10px] text-txt3"
            :class="t.role === 'human' ? 'text-right' : ''"
            data-testid="clarify-turn-bottom-time"
          >{{ locale && relTime(t.at) }}</div>
        </div>
      </div>
      <div v-if="thinking && !validating && liveAgentIdx < 0" class="flex items-center gap-2 pl-9 text-[12px] text-txt3">
        <span class="typing-dots"><i /><i /><i /></span>
        {{ translate('pages.clarify.thinking') }}
      </div>
    </div>
    <button
      v-if="showUnreadFab"
      type="button"
      data-testid="clarify-unread-fab"
      class="absolute bottom-3 left-1/2 z-10 inline-flex h-[34px] -translate-x-1/2 items-center gap-1.5 rounded-full bg-surface px-3.5 pl-3 text-[14px] font-semibold text-info shadow-[0_2px_10px_rgba(26,35,50,0.12)] transition-shadow hover:shadow-[0_4px_14px_rgba(26,35,50,0.16)]"
      :aria-label="unreadFabLabel"
      :title="unreadFabLabel"
      @click="onUnreadFabClick"
    >
      <Icon name="chevrons-down" :size="16" class="shrink-0" />
      <span class="min-w-[0.7em] text-center tabular-nums">{{ unreadCount }}</span>
    </button>
    </div>

    <div v-if="done" class="border-t border-line p-3 text-center text-[12px] text-ok">
      <Icon name="check" :size="13" class="-mt-0.5 mr-1 inline" />{{ translate('pages.clarify.done') }}
    </div>
    <div v-else-if="!active" class="border-t border-line p-3 text-center text-[12px] text-txt3">
      <Icon name="close" :size="13" class="-mt-0.5 mr-1 inline" />{{ translate('pages.clarify.closed') }}
    </div>
    <div v-else class="border-t border-line p-3">
      <!-- pending-send queue panel (Demo / AgentChatTester): clarify + review -->
      <div
        v-if="queued.length"
        class="mb-2 rounded-md border border-line bg-base/40 px-2.5 py-2"
        data-testid="clarify-review-queue"
      >
        <div class="mb-1 flex items-center gap-1.5 text-[11px] text-txt3">
          <Icon name="clock" :size="11" />
          {{ translate('pages.agentChatTester.queue', { n: queued.length }) }}
        </div>
        <div class="space-y-1">
          <div
            v-for="(q, qi) in queued"
            :key="q.id || qi"
            data-testid="clarify-queue-item"
            class="flex items-center gap-2 rounded border border-line bg-surface px-2 py-1 text-[12px] text-txt2"
          >
            <span class="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-line text-[9px] text-txt3">{{ qi + 1 }}</span>
            <span class="truncate">{{ q.text || (q.images.length || q.annotations.length ? '…' : '') }}</span>
          </div>
        </div>
      </div>
      <div v-if="showAnnotationChips && annotations.length" class="mb-2 flex flex-wrap gap-1.5">
        <AnnotationChip
          v-for="(a, ai) in annotations"
          :key="ai"
          :ann="a"
          removable
          test-id="clarify-annotation-chip"
          @remove="removeAnnotation(ai)"
        />
      </div>
      <div v-if="attachNotice" class="mb-2 border border-err/40 bg-err/10 px-2.5 py-1.5 text-[12px] text-err" data-testid="clarify-attach-notice" role="alert">
        {{ attachNotice }}
      </div>
      <div v-if="attachments.length" class="mb-2 flex flex-wrap gap-1.5">
        <div v-for="(im, ii) in attachments" :key="ii" class="relative">
          <ChatImageThumb
            v-if="isImageAttachment(im)"
            mode="locked"
            size="sm"
            thumb-class="rounded-md"
            :src="imgSrc(im)"
            :label="attachmentDisplayName(im, ii)"
            :alt="attachmentDisplayName(im, ii)"
            test-id="clarify-draft-image-thumb"
          />
          <div
            v-else
            class="flex h-14 max-w-[160px] items-center gap-1.5 border border-line bg-elevated px-2"
            :title="attachmentDisplayName(im, ii)"
            data-testid="clarify-pending-file-chip"
          >
            <span class="shrink-0 text-[9px] font-medium uppercase text-info">DOC</span>
            <span class="min-w-0 truncate text-[11px] text-txt2">{{ attachmentDisplayName(im, ii) }}</span>
          </div>
          <button
            class="absolute -right-1.5 -top-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-err text-white"
            @click.stop="removeAttachment(ii)"
          ><Icon name="close" :size="9" /></button>
        </div>
      </div>
      <div class="flex items-end gap-2">
        <input ref="fileInput" type="file" multiple class="hidden" @change="onPickFiles" />
        <button
          class="flex h-10 w-10 items-center justify-center rounded-md border border-line text-txt2 hover:border-line-strong disabled:opacity-50"
          :title="translate('pages.clarify.addImage')"
          data-testid="clarify-attach-btn"
          @click="fileInput?.click()"
        >
          <Icon name="paperclip" :size="16" />
        </button>
        <textarea
          ref="textareaRef"
          v-model="draft"
          class="input min-h-[40px] flex-1 resize-none"
          :class="overflowScroll ? 'scroll-area max-h-[128px] overflow-y-auto' : 'overflow-y-hidden'"
          rows="1"
          :placeholder="inputPlaceholder"
          data-testid="clarify-input"
          @input="onTextInput"
          @keydown="onComposerKeydown"
          @compositionstart="composing = true"
          @compositionend="composing = false"
          @paste="onPaste"
        />
        <button
          v-if="sendLabel"
          class="inline-flex h-10 items-center gap-1 rounded-md bg-accent px-3 text-xs font-semibold text-white hover:bg-accent-2 disabled:opacity-50"
          data-testid="clarify-send-label"
          :disabled="!draft.trim() && !attachments.length && !annotations.length"
          @click="send"
        >
          <Icon name="send" :size="14" /> {{ sendLabel }}
        </button>
        <button
          v-else
          class="flex h-10 w-10 items-center justify-center rounded-md bg-accent text-white hover:bg-accent-2 disabled:opacity-50"
          data-testid="clarify-send-icon"
          :disabled="!draft.trim() && !attachments.length && !annotations.length"
          @click="send"
        >
          <Icon name="send" :size="17" />
        </button>
        <button
          v-if="sessionBusy"
          type="button"
          class="inline-flex h-10 shrink-0 items-center gap-1 rounded-md border border-line bg-elevated px-3 text-xs font-semibold text-txt2 hover:border-line-strong"
          data-testid="clarify-review-cancel"
          title="Cancel"
          @click="cancelReview"
        >
          Cancel
        </button>
      </div>
      <div v-if="!hideFinish" class="mt-2 flex items-center justify-between gap-2">
        <p
          v-if="reviewMode"
          class="min-w-0 flex-1 text-[11px] leading-snug text-txt3"
          data-testid="clarify-confirm-hint"
        >
          {{ validating ? translate('pages.clarify.validating') : translate('pages.clarify.confirmFlowHint') }}
        </p>
        <span v-else class="flex-1" />
        <button
          v-if="reviewMode"
          class="inline-flex shrink-0 items-center gap-1 rounded-md bg-ok px-3 py-1.5 text-xs font-semibold text-white hover:bg-ok/90 disabled:opacity-50"
          data-testid="clarify-confirm-flow"
          :disabled="confirmDisabled"
          :title="translate('pages.clarify.confirmFlowTitle')"
          @click="finishEarly"
        >
          <Icon name="check" :size="13" />
          {{ validating ? translate('pages.clarify.validating') : translate('pages.clarify.confirmFlow') }}
        </button>
        <button
          v-else
          class="inline-flex shrink-0 items-center gap-1 rounded-md border border-line bg-elevated px-2.5 py-1 text-xs font-medium text-txt2 hover:border-line-strong disabled:opacity-50"
          :disabled="confirmDisabled"
          :title="translate('pages.clarify.finishEarlyTitle')"
          @click="finishEarly"
        >
          <Icon name="check" :size="13" /> {{ translate('pages.clarify.finishEarly') }}
        </button>
      </div>
    </div>
    <div
      v-if="reviewMode && confirmError && !done"
      class="flex items-center gap-1.5 border-t border-err/30 bg-err/10 px-3 py-2 text-[12px] text-err"
      data-testid="clarify-confirm-error"
      role="alert"
    >
      <Icon name="alert" :size="13" />
      <span class="min-w-0 flex-1 [overflow-wrap:anywhere]">{{ confirmError }}</span>
    </div>
  </div>

  <!-- Human history attachment image preview (single slot; no gallery / Esc) -->
  <ChatImagePreviewModal
    :open="!!imagePreview"
    :src="imagePreview?.src || ''"
    :label="imagePreview?.label || ''"
    test-id-prefix="clarify-image-preview"
    @close="closeImagePreview"
  />
</template>

<style scoped>
/* Card-deck step transition: subtle slide + fade as one card advances. */
.deck-enter-active,
.deck-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}
.deck-enter-from {
  opacity: 0;
  transform: translateX(8px);
}
.deck-leave-to {
  opacity: 0;
  transform: translateX(-8px);
}

/* "thinking" typing indicator: three dots bouncing in sequence. */
.typing-dots {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.typing-dots i {
  width: 5px;
  height: 5px;
  border-radius: 9999px;
  background: #22d3ee;
  animation: typing-bounce 1.2s infinite ease-in-out both;
}
.typing-dots i:nth-child(2) {
  animation-delay: 0.16s;
}
.typing-dots i:nth-child(3) {
  animation-delay: 0.32s;
}
@keyframes typing-bounce {
  0%,
  70%,
  100% {
    transform: translateY(0);
    opacity: 0.35;
  }
  35% {
    transform: translateY(-4px);
    opacity: 1;
  }
}

/* 「输出中」: same --grad-logo shimmer as .brand-logo__name */
.clarify-outputting {
  color: rgb(var(--c-accent-2));
  background: var(--grad-logo);
  background-size: var(--grad-logo-size);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  animation: shimmer 3.5s ease-in-out infinite;
}

/* Streaming caret at message tail (Demo phase 3) */
.clarify-stream-caret {
  display: inline-block;
  width: 7px;
  height: 1em;
  margin-left: 2px;
  vertical-align: text-bottom;
  background: rgb(var(--c-accent));
  animation: clarify-caret-blink 0.9s step-end infinite;
}
@keyframes clarify-caret-blink {
  50% {
    opacity: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .typing-dots i,
  .clarify-outputting,
  .clarify-stream-caret {
    animation: none !important;
  }
  .clarify-outputting {
    color: rgb(var(--c-accent-2));
    background: none;
    -webkit-background-clip: unset;
    background-clip: unset;
    -webkit-text-fill-color: unset;
  }
}
</style>

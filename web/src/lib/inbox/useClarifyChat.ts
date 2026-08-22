import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderMarkdown } from '@/lib/shared/markdown'
import { createStreamMarkdownPreview } from '@/lib/shared/streamMarkdownPreview'
import { createStreamTextReveal } from '@/lib/run/streamTextReveal'
import { mergePersistedAndLiveTurns, persistedCompletedLiveHuman } from '@/lib/inbox/mergeClarifyLiveTurns'
import { relTime } from '@/lib/shared/format'
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
import type { Ref } from 'vue'

export type ClarifyChatProps = {
  runId: string
  nodeId: string
  iteration: number
  turns?: import('@/lib/shared/types').ClarifyTurn[] | null
  done: boolean
  active?: boolean
  reviewMode?: boolean
  annotateEnabled?: boolean
  hideFinish?: boolean
  sendLabel?: string
  confirmError?: string | null
  nodeType?: string
  seedHumanText?: string
  seedHumanImages?: ClarifyImage[]
}

export type ClarifyChatEmit = {
  (e: 'send', text: string, images: ClarifyImage[], annotations: ReactAnnotation[]): void
  (e: 'finish'): void
  (e: 'cancel'): void
}

export type ClarifyChatModels = {
  draft: Ref<string>
  attachments: Ref<ClarifyImage[]>
  annotations: Ref<ReactAnnotation[]>
}

export function useClarifyChat(
  props: ClarifyChatProps,
  emit: ClarifyChatEmit,
  models: ClarifyChatModels,
) {
const draft = models.draft
const attachments = models.attachments
const annotations = models.annotations

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

const showAnnotationChips = computed(() => props.reviewMode || props.annotateEnabled)

const { t: translate, locale } = useI18n()

const persistedTurns = computed(() => props.turns ?? [])

const inputPlaceholder = computed(() => {
  if (props.reviewMode) return translate('pages.clarify.reviewInputPlaceholder')
  if (props.nodeType === 'approve') return translate('pages.clarify.approveInputPlaceholder')
  return translate('pages.clarify.inputPlaceholder')
})

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
    liveTurns.value.length === 0 &&
    queued.value.length === 0 &&
    !seedHumanTurn.value,
)
const useConfirmFlowAction = computed(
  () => props.reviewMode || props.nodeType === 'approve',
)

function humanMatchesSeed(t: ClarifyTurn, seed: ClarifyTurn): boolean {
  if (t.role !== 'human') return false
  const want = (seed.text || '').trim()
  if (want) return (t.text || '').trim() === want
  return (seed.images?.length || 0) > 0 && (t.images?.length || 0) > 0
}

const seedHumanTurn = computed<ClarifyTurn | null>(() => {
  const text = String(props.seedHumanText || '').trim()
  const images = props.seedHumanImages || []
  if (!text && images.length === 0) return null
  return { role: 'human', text, images, at: '' }
})

function prependSeedHuman(list: ClarifyTurn[]): ClarifyTurn[] {
  const seed = seedHumanTurn.value
  if (!seed) return list
  const seedImgs = seed.images || []
  const idx = list.findIndex((t) => humanMatchesSeed(t, seed))
  if (idx >= 0) {
    const t = list[idx]
    if (seedImgs.length && !(t.images && t.images.length)) {
      const next = list.slice()
      next[idx] = { ...t, images: seedImgs }
      return next
    }
    return list
  }
  return [seed, ...list]
}
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
    return prependSeedHuman(mergePersistedAndLiveTurns(list, liveTurns.value))
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
  return prependSeedHuman(list)
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
    // Clear live bubbles once persisted transcript includes them (not while streaming).
    // Empty persisted must not wipe an in-flight / seed-only human bubble.
    if (
      liveTurns.value.length &&
      (persistedCaughtUp || (turnList.length > 0 && !liveTurns.value.some((t) => t.streaming)))
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
// A rejected「确认并流转」must hand control back: both spinners are local state
// the server can no longer clear (the dialogue stays open, so props.done never
// flips), and leaving either on strands the user with a permanent placeholder.
watch(
  () => props.confirmError,
  (err) => {
    if (!err) return
    validating.value = false
    if (queued.value.length === 0 && liveAgentIdx.value < 0) thinking.value = false
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


  return {
    humanMatchesSeed,
    prependSeedHuman,
    isNearBottom,
    onScrollerScroll,
    scrollBottom,
    enterStickSequence,
    onUnreadFabClick,
    autoGrow,
    onTextInput,
    textHasChoicePrefix,
    stripChoicePrefix,
    questionKey,
    sessionStorageKey,
    readSessionChoice,
    writeSessionChoice,
    clearSessionChoice,
    isChoiceReply,
    latestQuestionTurnIndex,
    hasHumanReplyAfter,
    imagePreviewLabel,
    openImagePreview,
    turnsSemanticKey,
    addFiles,
    onPickFiles,
    onPaste,
    removeAttachment,
    sendMessage,
    sendFromComposer,
    onComposerKeydown,
    removeAnnotation,
    isActiveTurn,
    isSelected,
    toggle,
    answered,
    autoPickId,
    applyRecommended,
    submitRecommended,
    prevCard,
    nextCard,
    pick,
    submitChoices,
    parseChoiceSummary,
    choiceRowsForAgentTurn,
    selectedLabelsForQuestion,
    selectedDemoForInteractive,
    send,
    finishEarly,
    cancelReview,
    discardLastQueued,
    forceAuthoritativeIdle,
    settleAfterTurnEnd,
    applyQueueState,
    applyReviewFrame,
    applyAcpEvents,
    agentHasMessage,
    agentThoughtDisplay,
    isThoughtOpen,
    onThoughtToggle,
    showTurnCompleted,
    showAnnotationChips,
    persistedTurns,
    inputPlaceholder,
    thinking,
    validating,
    queued,
    liveTurns,
    showApproveEmptyHint,
    useConfirmFlowAction,
    seedHumanTurn,
    liveAgentIdx,
    liveStreamHtml,
    streamPreview,
    unsubStream,
    liveThoughtText,
    thoughtPreview,
    unsubThought,
    syncReveal,
    messageReveal,
    thoughtReveal,
    thoughtOpenOverride,
    scroller,
    fileInput,
    textareaRef,
    overflowScroll,
    composing,
    STICK_THRESHOLD,
    stickToBottom,
    unreadCount,
    showUnreadFab,
    unreadFabLabel,
    sessionBusy,
    AUTO_GROW_MIN,
    AUTO_GROW_MAX,
    pending,
    LEGACY_CHOICE_PREFIX,
    choicePrefix,
    latestQuestionIdx,
    latestQuestions,
    latestQuestionAnswered,
    displayTurns,
    attachNotice,
    sel,
    other,
    activeQuestions,
    someAnswered,
    hasRecommended,
    step,
    curQuestion,
    isFirstCard,
    isLastCard,
    confirmDisabled,
    translate,
    locale,
    imagePreview,
    openChatImagePreview,
    closeImagePreview,
    renderMarkdown,
    relTime,
    demoGridColsClass,
    demoOptionsOf,
    selectedDemoOption,
    useSideBySide,
    isSessionBusy: () => sessionBusy.value,
  }
}

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import HtmlPreview from '@/components/ui/HtmlPreview.vue'
import Icon from '@/components/ui/Icon.vue'
import AppModal from '@/components/ui/AppModal.vue'
import ReviewShell from '@/components/run/ReviewShell.vue'
import ClarifyChat from '@/components/run/ClarifyChat.vue'
import StructuredArtifactView from '@/components/run/StructuredArtifactView.vue'
import { applyPublicLocale } from '@/lib/shared/locale'
import { reapplyThemeChrome } from '@/lib/shared/theme'
import { useBreakpoint } from '@/lib/composables/useBreakpoint'
import { provideReviewAnnotate } from '@/lib/inbox/reviewAnnotate'
import { isAbortError } from '@/lib/run/liveLogRehydrate'
import {
  formatRemainingSec,
  parseShareTokenFromHash,
  publicGateApi,
  type PublicGateActiveItem,
  type PublicGateDecideResult,
  type PublicGatePreview,
  type PublicGateQueueItem,
} from '@/lib/inbox/gateShareLink'
import type { ClarifyImage, ClarifyTurn, ReactAnnotation } from '@/lib/shared/types'

type PublicChatRef = {
  discardLastQueued?: () => void
  applyQueueState?: (
    waiting: number,
    items: PublicGateQueueItem[] | null,
    busy?: boolean,
    activeItem?: PublicGateActiveItem | null,
  ) => void
  isSessionBusy?: () => boolean
}

const POLL_MS = 2000

const { t } = useI18n()
const { isMobile } = useBreakpoint()

const ready = ref(false)
const loading = ref(true)
const maybeStuck = ref(false)
const token = ref('')
const preview = ref<PublicGatePreview | null>(null)
const comment = ref('')
const reviewerName = ref('')
const submitting = ref(false)
const pendingKind = ref<'confirm' | 'reject' | null>(null)
const errorText = ref('')
const networkFailed = ref(false)
const workbenchSeen = ref(false)
const linkInvalid = ref(false)
const doneKind = ref<'approved' | 'rejected' | 'confirmed' | null>(null)
const upstreamOpen = ref(false)
const draft = ref('')
const attachments = ref<ClarifyImage[]>([])
const annotations = ref<ReactAnnotation[]>([])
const chatRef = ref<PublicChatRef | null>(null)
const replyInFlight = ref(false)
const pendingReplyText = ref('')

let previewGen = 0
let previewAbort: AbortController | null = null
let decideAbort: AbortController | null = null
let stuckTimer: number | null = null

function clearStuckTimer() {
  if (stuckTimer != null) {
    clearTimeout(stuckTimer)
    stuckTimer = null
  }
  maybeStuck.value = false
}

function abortPreview() {
  previewAbort?.abort()
  previewAbort = null
  clearStuckTimer()
}

const isReview = computed(() => preview.value?.kind === 'review')
const status = computed(() => preview.value?.status || (token.value ? 'invalid' : 'invalid'))
const isActive = computed(() => status.value === 'active')
const remainingLabel = computed(() => formatRemainingSec(preview.value?.remainingSec, t))
const reactAlive = computed(() => !!preview.value?.reactSessionAlive)
const sessionBusy = computed(() => !!preview.value?.sessionBusy)
const canReply = computed(() => isActive.value && reactAlive.value && !!preview.value?.actions?.reply)
const canReject = computed(() => !isReview.value && !!preview.value?.actions?.reject)
const canConfirm = computed(() => {
  if (!isActive.value || doneKind.value) return false
  if (sessionBusy.value || replyInFlight.value) return false
  if (isReview.value) return !!preview.value?.actions?.confirm
  return !!preview.value?.actions?.approve || !!preview.value?.actions?.confirm
})
const productKind = computed(() => preview.value?.productKind || inferProductKind())
const productName = computed(() => preview.value?.productName || preview.value?.structured?.name || '')
const inspectable = computed(() => isActive.value && reactAlive.value && productKind.value === 'visual')
const turns = computed<ClarifyTurn[]>(() =>
  (preview.value?.turns || []).map((turn) => ({
    role: turn.role === 'human' ? 'human' : 'agent',
    text: turn.text || '',
    at: turn.at || '',
    interrupted: !!turn.interrupted,
    annotations: (turn.annotations || []).map((a) => ({
      selector: a.selector,
      jsonPath: a.jsonPath,
      label: a.label,
      note: a.note,
      quote: a.quote,
    })),
  })),
)
const structuredDoc = computed(() => preview.value?.structured?.doc || structuredFallbackDoc())
const upstreamDoc = computed(() => preview.value?.upstream?.doc || null)
const hasUpstream = computed(() => !!preview.value?.upstream)
const statusHint = computed(() => {
  if (status.value === 'expired') return t('pages.publicGate.expiredHint')
  if (status.value === 'used') return t('pages.publicGate.usedHint')
  if (status.value === 'revoked') return t('pages.publicGate.revokedHint')
  return t('pages.publicGate.invalidHint')
})
const productLabel = computed(() => {
  if (productKind.value === 'visual') return t('pages.publicGate.visualProduct')
  if (productKind.value === 'app_preview') return t('pages.publicGate.appPreviewProduct')
  if (productKind.value === 'structured') return t('pages.publicGate.structuredProduct')
  return t('pages.publicGate.structuredProduct')
})

provideReviewAnnotate({
  get enabled() {
    return inspectable.value
  },
  annotate(ann) {
    const next: ReactAnnotation = {
      selector: ann.selector,
      jsonPath: ann.jsonPath,
      label: ann.label,
      quote: ann.quote,
      truncated: ann.truncated,
    }
    if (annotations.value.some((a) => a.selector === next.selector && a.jsonPath === next.jsonPath && a.quote === next.quote)) {
      return
    }
    annotations.value = [...annotations.value, next]
  },
})

function inferProductKind(): string {
  if (preview.value?.visualHtml) return 'visual'
  if (preview.value?.structured) return 'structured'
  return ''
}

function structuredFallbackDoc(): Record<string, unknown> | null {
  const s = preview.value?.structured
  if (!s) return null
  if (s.doc) return s.doc
  const out: Record<string, unknown> = {}
  if (s.title) out.title = s.title
  if (s.description) out.description = s.description
  if (s.goals) out.goals = s.goals
  if (s.text) out.summary = s.text
  return Object.keys(out).length ? out : null
}

async function loadPreview(opts?: { silent?: boolean }) {
  if (doneKind.value) return
  const attemptGen = ++previewGen
  abortPreview()
  previewAbort = new AbortController()
  const signal = previewAbort.signal

  if (!opts?.silent) {
    preview.value = null
    loading.value = true
    errorText.value = ''
    networkFailed.value = false
    workbenchSeen.value = false
    linkInvalid.value = false
    stuckTimer = window.setTimeout(() => {
      if (attemptGen === previewGen) maybeStuck.value = true
    }, 10_000)
  }
  const tok = parseShareTokenFromHash(window.location.hash)
  token.value = tok
  if (!tok) {
    if (attemptGen !== previewGen) return
    preview.value = { status: 'invalid' }
    loading.value = false
    clearStuckTimer()
    return
  }
  try {
    const next = await publicGateApi.preview(tok, signal)
    if (attemptGen !== previewGen) return
    preview.value = next
    if (next.status === 'active') workbenchSeen.value = true
    noteWorkbenchLinkInvalid(next)
    if (!opts?.silent && !linkInvalid.value) errorText.value = ''
  } catch (e) {
    if (attemptGen !== previewGen || isAbortError(e)) return
    if (!opts?.silent) {
      preview.value = null
      networkFailed.value = true
      errorText.value = t('pages.publicGate.networkError')
    }
  } finally {
    if (attemptGen === previewGen) {
      loading.value = false
      clearStuckTimer()
    }
  }
  if (attemptGen !== previewGen) return
  await nextTick()
  syncChatQueueFromPreview()
}

function turnsIncludeHumanText(text: string): boolean {
  const want = text.trim()
  if (!want) return false
  return turns.value.some((turn) => turn.role === 'human' && (turn.text || '').trim() === want)
}

function syncChatQueueFromPreview() {
  const chat = chatRef.value
  const p = preview.value
  if (!chat || !p || !isActive.value) return
  const waiting = typeof p.waiting === 'number' ? p.waiting : 0
  const items = p.queueItems || []
  const activeItem = p.activeItem || null
  const busy = !!p.sessionBusy || waiting > 0 || !!activeItem

  if (busy) {
    chat.applyQueueState?.(waiting, items.length ? items : null, true, activeItem)
    if (pendingReplyText.value && (turnsIncludeHumanText(pendingReplyText.value) || activeItem?.text?.trim() === pendingReplyText.value)) {
      pendingReplyText.value = ''
    }
    return
  }

  if (replyInFlight.value) return
  if (pendingReplyText.value && !turnsIncludeHumanText(pendingReplyText.value)) return

  if (pendingReplyText.value && turnsIncludeHumanText(pendingReplyText.value)) {
    chat.discardLastQueued?.()
    pendingReplyText.value = ''
  }
  chat.applyQueueState?.(0, [], false, null)
}

function clearHash() {
  try {
    history.replaceState(null, '', `${window.location.pathname}${window.location.search}`)
  } catch {
    // ignore
  }
}

function auditReady(): boolean {
  if (isReview.value) return true
  if (!reviewerName.value.trim() || !comment.value.trim()) {
    errorText.value = t('pages.publicGate.auditRequired')
    return false
  }
  return true
}

type DecideFailure = {
  status?: number
  body?: { error?: string; status?: string; message?: string }
  message?: string
  name?: string
}

function decideFailureOf(e: unknown): DecideFailure {
  if (!e || typeof e !== 'object') return { message: String(e || '') }
  const err = e as DecideFailure & { message?: string; name?: string }
  return {
    status: err.status,
    body: err.body,
    message: err.message || '',
    name: err.name,
  }
}

function isNonceError(e: unknown): boolean {
  return String(decideFailureOf(e).body?.error || '').toLowerCase() === 'nonce'
}

function isLinkInvalidStatus(status?: string, error?: string): boolean {
  const s = String(status || '').toLowerCase()
  const err = String(error || '').toLowerCase()
  if (['invalid', 'expired', 'revoked', 'used'].includes(s)) return true
  if (err === 'conflict' || ['invalid', 'expired', 'revoked', 'used'].includes(err)) return true
  return false
}

function noteWorkbenchLinkInvalid(p?: PublicGatePreview | null) {
  if (!workbenchSeen.value || doneKind.value || !p) return
  const expiredRemain = typeof p.remainingSec === 'number' && p.remainingSec <= 0
  if ((p.status && p.status !== 'active') || expiredRemain) {
    linkInvalid.value = true
    errorText.value = t('pages.publicGate.linkInvalid')
  }
}

function mapDecideFootnote(e: unknown): { key: string; linkInvalid: boolean } {
  const f = decideFailureOf(e)
  const code = String(f.body?.error || '').toLowerCase()
  const st = String(f.body?.status || '').toLowerCase()
  const msg = String(f.message || '')
  if (f.status === 429 || code === 'rate_limited') {
    return { key: 'pages.publicGate.rateLimited', linkInvalid: false }
  }
  if (f.status === 409 || isLinkInvalidStatus(st, code)) {
    return { key: 'pages.publicGate.linkInvalid', linkInvalid: true }
  }
  const networkLike =
    (typeof f.status === 'number' && f.status >= 500) ||
    f.name === 'TypeError' ||
    /failed to fetch|networkerror|network request failed|timeout|timed out/i.test(msg)
  if (networkLike) {
    return { key: 'pages.publicGate.networkFault', linkInvalid: false }
  }
  return { key: 'pages.publicGate.securityCheckFailed', linkInvalid: false }
}

function markLinkInvalid(status?: string) {
  linkInvalid.value = true
  errorText.value = t('pages.publicGate.linkInvalid')
  if (preview.value && status) {
    preview.value = { ...preview.value, status }
  }
}

async function applyDecideResult(kind: 'confirm' | 'reject', res: PublicGateDecideResult) {
  if (res.status === 'confirmed' || (res.alreadyProcessed && kind === 'confirm' && isReview.value)) {
    doneKind.value = 'confirmed'
    clearHash()
    return
  }
  if (res.status === 'approved' || (res.alreadyProcessed && kind === 'confirm' && !isReview.value)) {
    doneKind.value = 'approved'
    clearHash()
    return
  }
  if (res.status === 'rejected' || (res.alreadyProcessed && kind === 'reject')) {
    doneKind.value = 'rejected'
    clearHash()
    return
  }
  if (res.status === 'busy' || res.error === 'review_busy') {
    errorText.value = t('pages.publicGate.busy')
    await loadPreview({ silent: true })
    return
  }
  if (res.status === 'validation_failed' || res.error === 'review_validation_failed') {
    errorText.value = t('pages.publicGate.validationFailed')
    await loadPreview({ silent: true })
    return
  }
  if (isLinkInvalidStatus(res.status, res.error) || res.error === 'conflict') {
    markLinkInvalid(res.status || 'used')
    return
  }
  errorText.value = t('pages.publicGate.securityCheckFailed')
}

async function decideOnce(
  kind: 'confirm' | 'reject',
  nonce: string,
  signal: AbortSignal,
): Promise<PublicGateDecideResult> {
  const action =
    kind === 'reject'
      ? preview.value?.actions?.reject
      : preview.value?.actions?.confirm || preview.value?.actions?.approve || 'confirm'
  return publicGateApi.decide(
    {
      token: token.value,
      action: action || 'confirm',
      comment: isReview.value ? undefined : comment.value,
      name: isReview.value ? undefined : reviewerName.value,
      nonce,
    },
    signal,
  )
}

async function submitFinal(kind: 'confirm' | 'reject') {
  if (!preview.value || submitting.value || (linkInvalid.value && kind === 'confirm')) return
  if (sessionBusy.value && kind === 'confirm') {
    errorText.value = t('pages.publicGate.busy')
    return
  }
  if (!auditReady() && kind === 'reject') return
  if (!isReview.value && !auditReady()) return
  if (!preview.value.nonce) {
    errorText.value = t('pages.publicGate.unavailable')
    return
  }
  const action =
    kind === 'reject'
      ? preview.value.actions?.reject
      : preview.value.actions?.confirm || preview.value.actions?.approve || 'confirm'
  if (!action) return
  submitting.value = true
  pendingKind.value = kind
  errorText.value = ''
  stopPoll()
  abortPreview()
  decideAbort?.abort()
  decideAbort = new AbortController()
  const signal = decideAbort.signal
  try {
    let res: PublicGateDecideResult
    try {
      res = await decideOnce(kind, preview.value.nonce, signal)
    } catch (e) {
      if (isAbortError(e)) return
      if (kind !== 'confirm' || !isNonceError(e)) throw e
      await loadPreview({ silent: true })
      if (signal.aborted) return
      const nextNonce = preview.value?.nonce
      if (!nextNonce) throw e
      res = await decideOnce(kind, nextNonce, signal)
    }
    await applyDecideResult(kind, res)
  } catch (e) {
    if (isAbortError(e)) return
    const mapped = mapDecideFootnote(e)
    errorText.value = t(mapped.key)
    if (mapped.linkInvalid) {
      linkInvalid.value = true
    }
  } finally {
    submitting.value = false
    pendingKind.value = null
    if (isActive.value && !doneKind.value && !linkInvalid.value) startPoll()
  }
}

async function onSend(text: string, images: ClarifyImage[], anns: ReactAnnotation[]) {
  errorText.value = ''
  replyInFlight.value = true
  pendingReplyText.value = text.trim()
  try {
    await publicGateApi.reply({
      token: token.value,
      text,
      annotations: anns,
      images: images.map((im) => ({ data: im.data, mimeType: im.mimeType, name: im.name })),
    })
    await loadPreview({ silent: true })
  } catch (e) {
    chatRef.value?.discardLastQueued?.()
    pendingReplyText.value = ''
    errorText.value = e instanceof Error ? e.message : t('pages.publicGate.replyFailed')
  } finally {
    replyInFlight.value = false
    await nextTick()
    syncChatQueueFromPreview()
  }
}

async function onCancel() {
  errorText.value = ''
  try {
    await publicGateApi.cancel(token.value)
    await loadPreview({ silent: true })
  } catch (e) {
    errorText.value = e instanceof Error ? e.message : t('pages.publicGate.cancelFailed')
  }
}

function onHtmlPick(payload: { selector: string; tagName: string }) {
  if (!inspectable.value) return
  const next: ReactAnnotation = { selector: payload.selector, label: payload.selector || payload.tagName }
  if (annotations.value.some((a) => a.selector === next.selector)) return
  annotations.value = [...annotations.value, next]
}

let pollTimer: ReturnType<typeof setInterval> | null = null
function onHashChange() {
  if (doneKind.value || submitting.value) return
  void loadPreview()
}
function startPoll() {
  stopPoll()
  pollTimer = setInterval(() => {
    if (!isActive.value || doneKind.value || !token.value || submitting.value || linkInvalid.value) return
    void loadPreview({ silent: true })
  }, POLL_MS)
}
function stopPoll() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

watch([isActive, doneKind], () => {
  if (isActive.value && !doneKind.value && !submitting.value && !linkInvalid.value) startPoll()
  else stopPoll()
})

watch(chatRef, (chat) => {
  if (chat) syncChatQueueFromPreview()
})

onMounted(async () => {
  await applyPublicLocale()
  ready.value = true
  await loadPreview()
  window.addEventListener('hashchange', onHashChange)
  if (isActive.value) startPoll()
})
onUnmounted(() => {
  stopPoll()
  window.removeEventListener('hashchange', onHashChange)
  abortPreview()
  decideAbort?.abort()
  decideAbort = null
  reapplyThemeChrome()
})

defineExpose({ loadPreview })
</script>

<template>
  <div
    class="flex min-h-screen flex-col bg-base text-txt"
    data-testid="public-gate-root"
    :aria-busy="(!ready || loading || submitting) ? 'true' : 'false'"
  >
    <header
      class="flex shrink-0 items-center justify-between border-b border-line bg-surface px-4 py-2 text-txt"
      data-testid="public-gate-chrome"
    >
      <div class="flex items-center gap-2">
        <span
          class="border border-line bg-elevated px-2 py-0.5 text-[11px] text-txt2"
          data-testid="public-gate-badge"
        >
          {{ isReview ? t('pages.publicGate.badgeReview') : t('pages.publicGate.badge') }}
        </span>
        <span
          v-if="isActive && !doneKind && !reactAlive"
          class="text-[11px] text-txt3"
          data-testid="public-gate-session-ended"
        >
          {{ t('pages.publicGate.sessionEnded') }}
        </span>
      </div>
      <span v-if="isActive && !doneKind" class="text-[12px] text-txt3" data-testid="public-gate-remaining">
        {{ t('pages.publicGate.remaining', { remaining: remainingLabel }) }}
      </span>
    </header>

    <div
      v-if="!ready || loading"
      class="flex flex-1 flex-col items-center justify-center gap-3 py-16 text-center"
      role="status"
      aria-busy="true"
      data-testid="public-gate-loading"
    >
      <Icon name="spinner" :size="28" class="animate-spin text-accent" aria-hidden="true" />
      <p class="text-sm text-txt3">{{ t('pages.publicGate.loading') }}</p>
      <p v-if="maybeStuck" class="max-w-[40ch] text-xs text-txt3" data-testid="public-gate-maybe-stuck">
        {{ t('pages.publicGate.maybeStuck') }}
      </p>
    </div>

    <div
      v-else-if="networkFailed"
      class="flex flex-1 flex-col items-center justify-center gap-3 py-16 text-center"
      data-testid="public-gate-network-error"
      role="alert"
    >
      <Icon name="alert" :size="28" class="text-warn" />
      <h1 class="text-lg font-semibold">{{ t('pages.publicGate.networkError') }}</h1>
      <button
        type="button"
        class="inline-flex min-h-11 items-center justify-center border border-line bg-surface px-4 text-sm font-medium text-txt"
        data-testid="public-gate-network-retry"
        @click="loadPreview()"
      >
        {{ t('common.buttons.retry') }}
      </button>
    </div>

    <div
      v-else-if="doneKind"
      class="flex flex-1 flex-col items-center justify-center gap-3 py-16 text-center"
      data-testid="public-gate-done"
    >
      <Icon :name="doneKind === 'rejected' ? 'alert' : 'check'" :size="28" :class="doneKind === 'rejected' ? 'text-warn' : 'text-ok'" />
      <h1 class="text-lg font-semibold">
        {{
          doneKind === 'confirmed'
            ? t('pages.publicGate.doneConfirmed')
            : doneKind === 'approved'
              ? t('pages.publicGate.doneApproved')
              : t('pages.publicGate.doneRejected')
        }}
      </h1>
      <p class="text-sm text-txt3">{{ t('pages.publicGate.doneHint') }}</p>
    </div>

    <div
      v-else-if="!isActive && !workbenchSeen"
      class="flex flex-1 flex-col items-center justify-center gap-2 py-16 text-center"
      data-testid="public-gate-invalid"
      role="status"
    >
      <Icon name="alert" :size="28" class="text-warn" />
      <h1 class="text-lg font-semibold">
        {{
          status === 'expired'
            ? t('pages.publicGate.expired')
            : status === 'used'
              ? t('pages.publicGate.used')
              : status === 'revoked'
                ? t('pages.publicGate.revoked')
                : t('pages.publicGate.invalid')
        }}
      </h1>
      <p class="max-w-[40ch] text-sm text-txt3">{{ statusHint }}</p>
    </div>

    <div v-else class="flex min-h-0 flex-1 flex-col" data-testid="public-gate-workbench">
      <ReviewShell class="min-h-0 flex-1" :mobile="isMobile">
        <template #stage>
          <section class="flex min-h-0 flex-1 flex-col" data-testid="public-gate-stage">
            <div class="flex shrink-0 items-baseline gap-2 border-b border-line px-4 py-2">
              <h2 class="text-sm font-semibold" data-testid="public-gate-product-label">{{ productLabel }}</h2>
              <span v-if="productName" class="text-[11px] text-txt3" data-testid="public-gate-product-name">{{ productName }}</span>
            </div>
            <div class="min-h-0 flex-1 overflow-hidden">
              <div v-if="preview?.visualHtml" class="h-full min-h-[200px]" data-testid="public-gate-visual">
                <HtmlPreview
                  :html="preview.visualHtml"
                  mode="inline"
                  fill-parent
                  :enlargeable="false"
                  :inspectable="inspectable"
                  @pick="onHtmlPick"
                />
              </div>
              <div
                v-else-if="productKind === 'app_preview'"
                class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center text-sm text-txt3"
                data-testid="public-gate-app-preview"
              >
                <Icon name="monitor" :size="22" class="text-txt3" />
                <p>{{ t('pages.publicGate.appPreviewHint') }}</p>
              </div>
              <div
                v-else-if="structuredDoc"
                class="scroll-area h-full overflow-y-auto px-4 py-3"
                data-testid="public-gate-structured"
              >
                <StructuredArtifactView
                  :name="productName || preview?.structured?.name || 'research.json'"
                  :doc="structuredDoc"
                />
              </div>
              <div v-else class="flex h-full items-center justify-center text-sm text-txt3" data-testid="public-gate-empty-product">
                {{ t('pages.publicGate.emptyProduct') }}
              </div>
            </div>
          </section>
        </template>
        <template #sidebar>
          <div class="flex h-full min-h-0 flex-col" data-testid="public-gate-sidebar">
            <p
              v-if="!reactAlive"
              class="shrink-0 border-b border-line px-4 py-2 text-[11px] text-txt3"
              data-testid="public-gate-cold-hint"
            >
              {{ isReview ? t('pages.publicGate.sessionEndedHint') : t('pages.publicGate.sessionEndedHintGate') }}
            </p>
            <ClarifyChat
              ref="chatRef"
              class="min-h-0 flex-1"
              run-id="public-share"
              node-id="public-gate"
              :iteration="1"
              v-model:draft="draft"
              v-model:attachments="attachments"
              v-model:annotations="annotations"
              :turns="turns"
              :done="false"
              :active="canReply"
              review-mode
              annotate-enabled
              hide-finish
              @send="onSend"
              @cancel="onCancel"
            />
          </div>
        </template>
      </ReviewShell>

      <footer
        class="flex shrink-0 flex-col gap-2 border-t border-line bg-surface px-4 py-2.5 md:flex-row md:items-center"
        data-testid="public-gate-footer"
      >
        <div class="min-w-0 flex-1 text-xs text-txt2" data-testid="public-gate-upstream">
          <template v-if="hasUpstream">
            <span class="font-medium text-txt">{{ t('pages.gateApproval.upstreamContext') }}</span>
            <span class="text-txt3"> · {{ preview?.upstream?.summary || preview?.upstream?.title || t('pages.gateApproval.upstreamBarHint') }}</span>
          </template>
          <template v-else>
            <span class="text-txt3">{{ t('pages.publicGate.upstreamEmpty') }}</span>
          </template>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <button
            v-if="hasUpstream"
            type="button"
            class="inline-flex items-center gap-1.5 bg-accent px-2.5 py-1 text-[11px] font-medium text-white hover:bg-accent-2"
            data-testid="public-gate-upstream-enlarge"
            @click="upstreamOpen = true"
          >
            <Icon name="expand" :size="14" />
            {{ t('pages.gateApproval.upstreamEnlarge') }}
          </button>
          <template v-if="!isReview">
            <input
              v-model="reviewerName"
              type="text"
              maxlength="80"
              class="w-[8rem] border border-line bg-elevated px-2 py-1 text-xs text-txt"
              data-testid="public-gate-name"
              :placeholder="t('pages.publicGate.namePh')"
              autocomplete="name"
            />
            <input
              v-model="comment"
              type="text"
              maxlength="4000"
              class="min-w-[10rem] flex-1 border border-line bg-elevated px-2 py-1 text-xs text-txt md:w-[16rem] md:flex-none"
              data-testid="public-gate-comment"
              :placeholder="t('pages.publicGate.commentPh')"
            />
          </template>
          <span class="hidden text-[11px] text-txt3 md:inline" data-testid="public-gate-confirm-hint">
            {{ t('pages.publicGate.confirmHint') }}
          </span>
          <p v-if="errorText" class="text-xs text-err" role="alert" data-testid="public-gate-error">{{ errorText }}</p>
          <button
            v-if="canReject"
            type="button"
            class="inline-flex min-h-9 items-center gap-2 bg-transparent px-2 text-sm text-txt2 underline underline-offset-4 hover:text-txt disabled:opacity-45"
            data-testid="public-gate-reject"
            :disabled="submitting"
            :aria-busy="submitting ? 'true' : 'false'"
            :aria-label="t('pages.publicGate.rejectAria')"
            @click="submitFinal('reject')"
          >
            <Icon v-if="submitting" name="spinner" :size="16" class="animate-spin" aria-hidden="true" />
            {{ submitting ? t('pages.publicGate.submitting') : t('pages.publicGate.reject') }}
          </button>
          <button
            v-if="canConfirm || linkInvalid"
            type="button"
            class="inline-flex min-h-9 min-w-[8rem] items-center justify-center gap-2 bg-ok px-4 text-sm font-medium text-white disabled:opacity-45"
            data-testid="public-gate-confirm"
            :disabled="submitting || sessionBusy || replyInFlight || linkInvalid || !canConfirm"
            :aria-busy="pendingKind === 'confirm' && submitting ? 'true' : 'false'"
            :aria-label="t('pages.publicGate.confirmAria')"
            @click="submitFinal('confirm')"
          >
            <Icon v-if="pendingKind === 'confirm' && submitting" name="spinner" :size="16" class="animate-spin" aria-hidden="true" />
            {{ pendingKind === 'confirm' && submitting ? t('pages.publicGate.confirming') : t('pages.publicGate.confirm') }}
          </button>
        </div>
      </footer>
    </div>

    <AppModal
      :open="upstreamOpen"
      :title="t('pages.gateApproval.upstreamContext')"
      :width="720"
      close-on-esc
      data-testid="public-gate-upstream-modal"
      @close="upstreamOpen = false"
    >
      <div v-if="upstreamDoc" class="scroll-area max-h-[70vh] overflow-y-auto px-4 py-3">
        <StructuredArtifactView name="clarified_requirement.json" :doc="upstreamDoc" />
      </div>
      <div v-else class="space-y-2 px-4 py-3 text-sm text-txt2">
        <p v-if="preview?.upstream?.title" class="font-medium text-txt">{{ preview.upstream.title }}</p>
        <p v-if="preview?.upstream?.summary">{{ preview.upstream.summary }}</p>
        <p v-if="preview?.upstream?.description">{{ preview.upstream.description }}</p>
        <p v-if="preview?.upstream?.text">{{ preview.upstream.text }}</p>
      </div>
    </AppModal>
  </div>
</template>

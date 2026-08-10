<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import HtmlPreview from '@/components/ui/HtmlPreview.vue'
import Icon from '@/components/ui/Icon.vue'
import { renderMarkdown } from '@/lib/markdown'
import { applyPublicLocale } from '@/lib/locale'
import { applyPublicLightChrome, reapplyThemeChrome } from '@/lib/theme'
import { isAbortError } from '@/lib/liveLogRehydrate'
import {
  formatRemainingSec,
  parseShareTokenFromHash,
  publicGateApi,
  type PublicGatePreview,
} from '@/lib/gateShareLink'

const { t } = useI18n()

const ready = ref(false)
const loading = ref(true)
const maybeStuck = ref(false)
const token = ref('')
const preview = ref<PublicGatePreview | null>(null)
const comment = ref('')
const reviewerName = ref('')
const submitting = ref(false)
const errorText = ref('')
const networkFailed = ref(false)
const doneKind = ref<'approved' | 'rejected' | 'confirmed' | null>(null)
const isReview = computed(() => preview.value?.kind === 'review')

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

function sandboxPreviewError(): string {
  return t('pages.publicGate.networkError')
}

const status = computed(() => preview.value?.status || (token.value ? 'invalid' : 'invalid'))
const isActive = computed(() => status.value === 'active')
const canConfirm = computed(() => isReview.value && (!!preview.value?.actions?.confirm || isActive.value))
const remainingLabel = computed(() =>
  formatRemainingSec(preview.value?.remainingSec, t),
)
const descriptionHtml = computed(() => renderMarkdown(preview.value?.description || ''))
const canApprove = computed(() => !!preview.value?.actions?.approve)
const canReject = computed(() => !!preview.value?.actions?.reject)
const hasDeliverable = computed(
  () => !!preview.value?.visualHtml || !!preview.value?.structured,
)
const statusHint = computed(() => {
  if (status.value === 'expired') return t('pages.publicGate.expiredHint')
  if (status.value === 'used') return t('pages.publicGate.usedHint')
  if (status.value === 'revoked') return t('pages.publicGate.revokedHint')
  return t('pages.publicGate.invalidHint')
})

async function loadPreview() {
  if (doneKind.value) return
  const attemptGen = ++previewGen
  abortPreview()
  previewAbort = new AbortController()
  const signal = previewAbort.signal

  preview.value = null
  errorText.value = ''
  networkFailed.value = false
  doneKind.value = null
  loading.value = true
  maybeStuck.value = false
  stuckTimer = window.setTimeout(() => {
    if (attemptGen === previewGen) maybeStuck.value = true
  }, 10_000)

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
    errorText.value = ''
  } catch (e) {
    if (attemptGen !== previewGen) return
    if (isAbortError(e)) return
    preview.value = null
    networkFailed.value = true
    errorText.value = sandboxPreviewError()
  } finally {
    if (attemptGen === previewGen) {
      loading.value = false
      clearStuckTimer()
    }
  }
}

function clearHash() {
  try {
    history.replaceState(null, '', `${window.location.pathname}${window.location.search}`)
  } catch {
    // ignore
  }
}

async function submit(kind: 'approve' | 'reject') {
  if (!preview.value || submitting.value) return
  const action = kind === 'approve' ? preview.value.actions?.approve : preview.value.actions?.reject
  if (!action) return
  if (kind === 'reject' && !comment.value.trim()) {
    errorText.value = t('pages.publicGate.commentRequired')
    return
  }
  if (!preview.value.nonce) {
    errorText.value = t('pages.publicGate.unavailable')
    return
  }
  submitting.value = true
  errorText.value = ''
  decideAbort?.abort()
  decideAbort = new AbortController()
  try {
    const res = await publicGateApi.decide(
      {
        token: token.value,
        action,
        comment: comment.value,
        name: reviewerName.value,
        nonce: preview.value.nonce,
      },
      decideAbort.signal,
    )
    if (res.status === 'approved' || (res.alreadyProcessed && kind === 'approve')) {
      doneKind.value = 'approved'
      clearHash()
      return
    }
    if (res.status === 'rejected' || (res.alreadyProcessed && kind === 'reject')) {
      doneKind.value = 'rejected'
      clearHash()
      return
    }
    if (res.status === 'used' || res.error === 'conflict') {
      preview.value = { status: 'used' }
      return
    }
    if (res.status === 'expired') {
      preview.value = { status: 'expired' }
      return
    }
    if (res.status === 'revoked') {
      preview.value = { status: 'revoked' }
      return
    }
    preview.value = { status: res.status || 'invalid' }
  } catch (e) {
    if (isAbortError(e)) return
    errorText.value = sandboxPreviewError()
    try {
      const refreshed = await publicGateApi.preview(token.value, decideAbort.signal)
      if (refreshed.status && refreshed.status !== 'active') {
        preview.value = refreshed
        errorText.value = ''
      }
    } catch {
      // keep sandboxed network error
    }
  } finally {
    submitting.value = false
  }
}

async function submitReviewConfirm() {
  if (!preview.value || submitting.value) return
  if (!preview.value.nonce) {
    errorText.value = t('pages.publicGate.unavailable')
    return
  }
  submitting.value = true
  errorText.value = ''
  decideAbort?.abort()
  decideAbort = new AbortController()
  try {
    const res = await publicGateApi.decide(
      {
        token: token.value,
        action: preview.value.actions?.confirm || 'confirm',
        nonce: preview.value.nonce,
      },
      decideAbort.signal,
    )
    if (res.status === 'confirmed' || res.alreadyProcessed) {
      doneKind.value = 'confirmed'
      clearHash()
      return
    }
    if (res.status === 'busy' || res.error === 'review_busy') {
      errorText.value = t('pages.publicGate.busy')
      return
    }
    if (res.status === 'validation_failed' || res.error === 'review_validation_failed') {
      errorText.value = t('pages.publicGate.validationFailed')
      return
    }
    if (res.status === 'used' || res.error === 'conflict') {
      preview.value = { ...preview.value, status: 'used' }
      return
    }
    preview.value = { ...preview.value, status: res.status || 'invalid' }
  } catch (e) {
    if (isAbortError(e)) return
    errorText.value = sandboxPreviewError()
    try {
      const refreshed = await publicGateApi.preview(token.value, decideAbort.signal)
      if (refreshed.status && refreshed.status !== 'active') {
        preview.value = { ...preview.value, ...refreshed }
        errorText.value = ''
      }
    } catch {
      // keep sandboxed network error
    }
  } finally {
    submitting.value = false
  }
}

function onHashChange() {
  if (doneKind.value || submitting.value) return
  void loadPreview()
}

onMounted(async () => {
  applyPublicLightChrome()
  await applyPublicLocale()
  ready.value = true
  window.addEventListener('hashchange', onHashChange)
  await loadPreview()
})
onUnmounted(() => {
  window.removeEventListener('hashchange', onHashChange)
  abortPreview()
  decideAbort?.abort()
  decideAbort = null
  reapplyThemeChrome()
})
</script>

<template>
  <div
    class="flex min-h-screen flex-col bg-base text-txt"
    data-testid="public-gate-root"
    :aria-busy="(!ready || loading || submitting) ? 'true' : 'false'"
  >
    <header
      class="flex shrink-0 items-center justify-between bg-accent px-4 py-3 text-white"
      data-testid="public-gate-chrome"
    >
      <div class="flex items-center gap-2">
        <span class="text-sm font-semibold tracking-wide">Approving</span>
        <span
          class="border border-white/40 bg-white/15 px-2 py-0.5 text-[11px] text-white"
          data-testid="public-gate-badge"
        >
          {{ isReview ? t('pages.publicGate.badgeReview') : t('pages.publicGate.badge') }}
        </span>
      </div>
      <span v-if="isActive && !doneKind" class="text-[12px] text-white/80" data-testid="public-gate-remaining">
        {{ t('pages.publicGate.remaining', { remaining: remainingLabel }) }}
      </span>
    </header>

    <main class="mx-auto flex w-full flex-1 flex-col px-4 py-6 md:px-8">
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
          @click="loadPreview"
        >
          {{ t('common.chatImage.retry') }}
        </button>
      </div>

      <div v-else-if="doneKind" class="flex flex-1 flex-col items-center justify-center gap-3 py-16 text-center" data-testid="public-gate-done">
        <Icon
          :name="doneKind === 'rejected' ? 'alert' : 'check'"
          :size="28"
          :class="doneKind === 'rejected' ? 'text-warn' : 'text-ok'"
        />
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
        v-else-if="!isActive"
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

      <div v-else class="flex min-h-0 flex-1 flex-col gap-5">
        <div>
          <h1 class="text-xl font-semibold text-txt md:text-2xl" data-testid="public-gate-title">
            {{ t('pages.publicGate.heading') }}
          </h1>
          <p class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-txt2" data-testid="public-gate-meta">
            <span v-if="preview?.title" data-testid="public-gate-gate-title">{{ preview.title }}</span>
            <span>{{ t('pages.publicGate.onceOnly') }}</span>
            <span>{{ t('pages.publicGate.noLogin') }}</span>
            <span>{{ t('pages.publicGate.previewRedacted') }}</span>
          </p>
          <div
            v-if="preview?.description"
            class="prose-sm mt-2 text-txt2"
            data-testid="public-gate-desc"
            v-html="descriptionHtml"
          />
        </div>

        <section
          v-if="hasDeliverable"
          class="flex min-h-0 flex-1 flex-col gap-3"
          data-testid="public-gate-content"
        >
          <div>
            <h2 class="text-sm font-semibold text-txt">{{ t('pages.publicGate.contentLabel') }}</h2>
            <p class="mt-0.5 text-xs text-txt3">{{ t('pages.publicGate.contentHint') }}</p>
          </div>

          <div v-if="preview?.visualHtml" class="min-h-[200px] w-full border border-line bg-white" data-testid="public-gate-visual">
            <HtmlPreview
              :html="preview.visualHtml"
              mode="inline"
              :enlargeable="false"
              :inspectable="false"
            />
          </div>

          <div
            v-if="preview?.structured"
            class="border border-line bg-surface px-3 py-3"
            data-testid="public-gate-structured"
          >
            <div v-if="preview.structured.name" class="text-[11px] text-txt3">{{ preview.structured.name }}</div>
            <div v-if="preview.structured.title" class="mt-1 font-medium">{{ preview.structured.title }}</div>
            <ul v-if="Array.isArray(preview.structured.goals)" class="mt-2 list-disc pl-5 text-sm text-txt2">
              <li v-for="(g, i) in preview.structured.goals" :key="i">{{ g }}</li>
            </ul>
            <p v-else-if="preview.structured.text" class="mt-2 text-sm text-txt2">{{ preview.structured.text }}</p>
          </div>
        </section>

        <form class="mt-auto max-w-xl space-y-3 border-t border-line pt-4" @submit.prevent>
          <template v-if="isReview">
            <p v-if="errorText" class="text-xs text-err" role="alert" data-testid="public-gate-error">{{ errorText }}</p>
            <button
              v-if="canConfirm"
              type="button"
              class="inline-flex min-h-11 min-w-[44px] w-full items-center justify-center gap-2 bg-ok px-4 text-sm font-medium text-white disabled:opacity-45"
              data-testid="public-gate-confirm"
              :disabled="submitting"
              :aria-busy="submitting ? 'true' : 'false'"
              :aria-label="t('pages.publicGate.confirmAria')"
              @click="submitReviewConfirm"
            >
              <Icon v-if="submitting" name="spinner" :size="16" class="animate-spin" aria-hidden="true" />
              {{ submitting ? t('pages.publicGate.submitting') : t('pages.publicGate.confirm') }}
            </button>
          </template>
          <template v-else>
          <label class="block text-xs text-txt2">
            {{ t('pages.publicGate.nameLabel') }}
            <input
              v-model="reviewerName"
              type="text"
              maxlength="80"
              class="mt-1 w-full border border-line bg-elevated px-3 py-2 text-sm text-txt"
              data-testid="public-gate-name"
              :placeholder="t('pages.publicGate.namePh')"
              autocomplete="name"
            />
          </label>
          <label class="block text-xs text-txt2">
            {{ t('pages.publicGate.commentLabel') }}
            <textarea
              v-model="comment"
              rows="3"
              maxlength="4000"
              class="mt-1 w-full border border-line bg-elevated px-3 py-2 text-sm text-txt"
              data-testid="public-gate-comment"
              :placeholder="t('pages.publicGate.commentPh')"
            />
          </label>
          <p v-if="errorText" class="text-xs text-err" role="alert">{{ errorText }}</p>
          <div class="flex flex-col items-start gap-3">
            <button
              v-if="canApprove"
              type="button"
              class="inline-flex min-h-11 min-w-[12rem] items-center justify-center gap-2 bg-accent px-5 text-sm font-semibold text-white disabled:opacity-45"
              data-testid="public-gate-approve"
              :disabled="submitting"
              :aria-busy="submitting ? 'true' : 'false'"
              :aria-label="t('pages.publicGate.approveAria')"
              @click="submit('approve')"
            >
              <Icon v-if="submitting" name="spinner" :size="16" class="animate-spin" aria-hidden="true" />
              {{ submitting ? t('pages.publicGate.submitting') : t('pages.publicGate.approve') }}
            </button>
            <button
              v-if="canReject"
              type="button"
              class="inline-flex min-h-11 items-center gap-2 bg-transparent px-0 text-sm text-txt2 underline underline-offset-4 hover:text-txt disabled:opacity-45"
              data-testid="public-gate-reject"
              :disabled="submitting"
              :aria-busy="submitting ? 'true' : 'false'"
              :aria-label="t('pages.publicGate.rejectAria')"
              @click="submit('reject')"
            >
              <Icon v-if="submitting" name="spinner" :size="16" class="animate-spin" aria-hidden="true" />
              {{ submitting ? t('pages.publicGate.submitting') : t('pages.publicGate.reject') }}
            </button>
          </div>
          </template>
        </form>
      </div>
    </main>
  </div>
</template>

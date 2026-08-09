<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import HtmlPreview from '@/components/ui/HtmlPreview.vue'
import Icon from '@/components/ui/Icon.vue'
import { renderMarkdown } from '@/lib/markdown'
import { applyPublicLocale } from '@/lib/locale'
import {
  formatRemainingSec,
  parseShareTokenFromHash,
  publicGateApi,
  type PublicGatePreview,
} from '@/lib/gateShareLink'

const { t } = useI18n()

const ready = ref(false)
const loading = ref(true)
const token = ref('')
const preview = ref<PublicGatePreview | null>(null)
const comment = ref('')
const reviewerName = ref('')
const submitting = ref(false)
const errorText = ref('')
const doneKind = ref<'approved' | 'rejected' | null>(null)

const status = computed(() => preview.value?.status || (token.value ? 'invalid' : 'invalid'))
const isActive = computed(() => status.value === 'active')
const remainingLabel = computed(() =>
  formatRemainingSec(preview.value?.remainingSec, t),
)
const descriptionHtml = computed(() => renderMarkdown(preview.value?.description || ''))
const canApprove = computed(() => !!preview.value?.actions?.approve)
const canReject = computed(() => !!preview.value?.actions?.reject)

async function loadPreview() {
  loading.value = true
  errorText.value = ''
  const tok = parseShareTokenFromHash(window.location.hash)
  token.value = tok
  if (!tok) {
    preview.value = { status: 'invalid' }
    loading.value = false
    return
  }
  try {
    preview.value = await publicGateApi.preview(tok)
  } catch (e) {
    preview.value = { status: 'invalid' }
    errorText.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
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
  try {
    const res = await publicGateApi.decide({
      token: token.value,
      action,
      comment: comment.value,
      name: reviewerName.value,
      nonce: preview.value.nonce,
    })
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
    preview.value = { status: res.status || 'invalid' }
  } catch (e) {
    errorText.value = e instanceof Error ? e.message : String(e)
    try {
      preview.value = await publicGateApi.preview(token.value)
    } catch {
      // keep error
    }
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await applyPublicLocale()
  ready.value = true
  await loadPreview()
  window.addEventListener('hashchange', loadPreview)
})
onUnmounted(() => {
  window.removeEventListener('hashchange', loadPreview)
})
</script>

<template>
  <div
    class="flex min-h-screen flex-col bg-base text-txt"
    data-testid="public-gate-root"
  >
    <header class="flex shrink-0 items-center justify-between border-b border-line px-4 py-3">
      <div class="flex items-center gap-2">
        <span class="text-sm font-semibold tracking-wide text-txt">Approving</span>
        <span class="border border-accent/40 bg-accent/10 px-2 py-0.5 text-[11px] text-accent-2">
          {{ t('pages.publicGate.badge') }}
        </span>
      </div>
      <span v-if="isActive && !doneKind" class="text-[12px] text-txt3" data-testid="public-gate-remaining">
        {{ t('pages.publicGate.remaining', { remaining: remainingLabel }) }}
      </span>
    </header>

    <main class="mx-auto flex w-full max-w-3xl flex-1 flex-col px-4 py-6">
      <div v-if="!ready || loading" class="py-16 text-center text-sm text-txt3">
        {{ t('common.buttons.loading') }}
      </div>

      <div v-else-if="doneKind" class="flex flex-1 flex-col items-center justify-center gap-3 py-16 text-center" data-testid="public-gate-done">
        <Icon :name="doneKind === 'approved' ? 'check' : 'alert'" :size="28" :class="doneKind === 'approved' ? 'text-ok' : 'text-warn'" />
        <h1 class="text-lg font-semibold">
          {{ doneKind === 'approved' ? t('pages.publicGate.doneApproved') : t('pages.publicGate.doneRejected') }}
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
        <p class="max-w-[40ch] text-sm text-txt3">{{ t('pages.publicGate.invalidHint') }}</p>
      </div>

      <div v-else class="flex min-h-0 flex-1 flex-col gap-4">
        <div>
          <h1 class="text-lg font-semibold text-txt" data-testid="public-gate-title">{{ preview?.title }}</h1>
          <div
            v-if="preview?.description"
            class="prose-sm mt-2 text-txt2"
            data-testid="public-gate-desc"
            v-html="descriptionHtml"
          />
        </div>

        <div v-if="preview?.visualHtml" class="min-h-[200px] border border-line" data-testid="public-gate-visual">
          <HtmlPreview :html="preview.visualHtml" mode="default" :enlargeable="true" :inspectable="false" />
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

        <form class="mt-auto space-y-3 border-t border-line pt-4" @submit.prevent>
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
          <div class="flex flex-wrap gap-2">
            <button
              v-if="canApprove"
              type="button"
              class="inline-flex min-h-11 min-w-[44px] flex-1 items-center justify-center bg-ok px-4 text-sm font-medium text-white disabled:opacity-45"
              data-testid="public-gate-approve"
              :disabled="submitting"
              :aria-label="t('pages.publicGate.approveAria')"
              @click="submit('approve')"
            >
              {{ t('pages.publicGate.approve') }}
            </button>
            <button
              v-if="canReject"
              type="button"
              class="inline-flex min-h-11 min-w-[44px] flex-1 items-center justify-center border border-err/50 bg-err/10 px-4 text-sm font-medium text-err disabled:opacity-45"
              data-testid="public-gate-reject"
              :disabled="submitting"
              :aria-label="t('pages.publicGate.rejectAria')"
              @click="submit('reject')"
            >
              {{ t('pages.publicGate.reject') }}
            </button>
          </div>
        </form>
      </div>
    </main>
  </div>
</template>

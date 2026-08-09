<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppModal from '@/components/ui/AppModal.vue'
import Icon from '@/components/ui/Icon.vue'
import { api } from '@/lib/api'
import { useToast } from '@/lib/useToast'
import type { GateInboxItem, GateShareInboxStatus } from '@/lib/types'
import {
  DEFAULT_GATE_SHARE_TTL,
  GATE_SHARE_TTL_TIERS,
  canCreateGateShare,
  forgetShareUrl,
  isGateShareActive,
  maskShareUrl,
  recallShareUrl,
  rememberShareUrl,
  type GateShareTTLTier,
} from '@/lib/gateShareLink'

const props = defineProps<{
  open: boolean
  gate: GateInboxItem | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'updated', status: GateShareInboxStatus, url?: string): void
  (e: 'revoked', status: GateShareInboxStatus): void
}>()

const { t } = useI18n()
const toast = useToast()

const ttlTier = ref<GateShareTTLTier>(DEFAULT_GATE_SHARE_TTL)
const fullUrl = ref('')
const revealUrl = ref(false)
const busy = ref(false)
const confirmKind = ref<'regen' | 'revoke' | null>(null)
const localStatus = ref<GateShareInboxStatus | null>(null)
const errorText = ref('')

const status = computed(() => localStatus.value || props.gate?.shareLink || { state: 'none' })
const mode = computed(() => {
  if (status.value.state === 'used' || (!canCreateGateShare(status.value) && !isGateShareActive(status.value))) {
    return 'readonly' as const
  }
  if (isGateShareActive(status.value)) return 'manage' as const
  return 'create' as const
})
const displayUrl = computed(() => {
  if (!fullUrl.value) return ''
  return revealUrl.value ? fullUrl.value : maskShareUrl(fullUrl.value)
})

watch(
  () => [props.open, props.gate?.runId, props.gate?.nodeId] as const,
  () => {
    if (!props.open) {
      revealUrl.value = false
      fullUrl.value = ''
      confirmKind.value = null
      errorText.value = ''
      return
    }
    localStatus.value = props.gate?.shareLink || { state: 'none' }
    ttlTier.value =
      (props.gate?.shareLink?.ttlTier as GateShareTTLTier) || DEFAULT_GATE_SHARE_TTL
    revealUrl.value = false
    fullUrl.value = recallShareUrl(props.gate?.runId || '', props.gate?.nodeId || '', props.gate?.iteration)
    confirmKind.value = null
    errorText.value = ''
  },
  { immediate: true },
)

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('pages.gatesInbox.share.copied'))
    revealUrl.value = false
    return true
  } catch {
    revealUrl.value = true
    toast.info(t('pages.gatesInbox.share.clipboardFallback'))
    return false
  }
}

async function createAndCopy() {
  if (!props.gate || busy.value) return
  busy.value = true
  errorText.value = ''
  try {
    const res = await api.createGateShareLink(props.gate.runId, props.gate.nodeId, ttlTier.value)
    fullUrl.value = res.url
    rememberShareUrl(props.gate.runId, props.gate.nodeId, props.gate.iteration, res.url)
    const next: GateShareInboxStatus = {
      state: res.state || 'active',
      ttlTier: res.ttlTier,
      expiresAt: res.expiresAt,
      canCreate: false,
      canManage: true,
      remainingSec: undefined,
    }
    localStatus.value = next
    emit('updated', next, res.url)
    await copyText(res.url)
  } catch (e) {
    errorText.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}

async function copyExisting() {
  if (!fullUrl.value) {
    errorText.value = t('pages.gatesInbox.share.needRegenToCopy')
    return
  }
  await copyText(fullUrl.value)
}

async function confirmRegen() {
  if (!props.gate || busy.value) return
  busy.value = true
  errorText.value = ''
  try {
    const res = await api.regenGateShareLink(props.gate.runId, props.gate.nodeId)
    fullUrl.value = res.url
    rememberShareUrl(props.gate.runId, props.gate.nodeId, props.gate.iteration, res.url)
    revealUrl.value = false
    confirmKind.value = null
    const next: GateShareInboxStatus = {
      state: res.state || 'active',
      ttlTier: res.ttlTier,
      expiresAt: res.expiresAt,
      canCreate: false,
      canManage: true,
    }
    localStatus.value = next
    emit('updated', next, res.url)
    await copyText(res.url)
  } catch (e) {
    errorText.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}

async function confirmRevoke() {
  if (!props.gate || busy.value) return
  busy.value = true
  errorText.value = ''
  try {
    await api.revokeGateShareLink(props.gate.runId, props.gate.nodeId)
    forgetShareUrl(props.gate.runId, props.gate.nodeId, props.gate.iteration)
    const next: GateShareInboxStatus = {
      state: 'revoked',
      canCreate: true,
      canManage: false,
      ttlTier: status.value.ttlTier,
    }
    localStatus.value = next
    confirmKind.value = null
    emit('revoked', next)
    emit('close')
  } catch (e) {
    errorText.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}

function close() {
  emit('close')
}
</script>

<template>
  <AppModal
    :open="open"
    :title="t('pages.gatesInbox.share.panelTitle')"
    :width="520"
    close-on-esc
    @close="close"
  >
    <div v-if="gate" class="space-y-4 text-sm text-txt" data-testid="gate-share-panel-body">
      <p class="border border-warn/35 bg-warn/10 px-3 py-2 text-[13px] text-warn" role="note">
        {{ t('pages.gatesInbox.share.safetyHint') }}
      </p>

      <div v-if="mode === 'readonly'" class="space-y-2" role="status">
        <p class="text-txt2">{{ t('pages.gatesInbox.share.usedReadonly') }}</p>
      </div>

      <div v-else-if="mode === 'create'" class="space-y-3">
        <label class="block text-xs font-medium text-txt2" for="gate-share-ttl">
          {{ t('pages.gatesInbox.share.ttlLabel') }}
        </label>
        <div id="gate-share-ttl" class="flex flex-wrap gap-2" role="radiogroup" :aria-label="t('pages.gatesInbox.share.ttlLabel')">
          <button
            v-for="tier in GATE_SHARE_TTL_TIERS"
            :key="tier"
            type="button"
            class="min-h-11 border px-3 text-xs"
            :class="ttlTier === tier ? 'border-accent bg-accent text-white' : 'border-line text-txt2 hover:bg-elevated'"
            :aria-checked="ttlTier === tier ? 'true' : 'false'"
            role="radio"
            data-testid="gate-share-ttl"
            :data-tier="tier"
            @click="ttlTier = tier"
          >
            {{ t(`pages.gatesInbox.share.ttl.${tier}`) }}
          </button>
        </div>
        <button
          type="button"
          class="inline-flex min-h-11 w-full items-center justify-center gap-2 bg-accent px-3 text-sm font-medium text-white hover:bg-accent-2 disabled:opacity-45"
          data-testid="gate-share-create"
          :disabled="busy"
          :aria-label="t('pages.gatesInbox.share.createCopyAria')"
          @click="createAndCopy"
        >
          <Icon name="copy" :size="16" />
          {{ t('pages.gatesInbox.share.createCopy') }}
        </button>
      </div>

      <div v-else class="space-y-3">
        <label class="block text-xs font-medium text-txt2">{{ t('pages.gatesInbox.share.linkLabel') }}</label>
        <textarea
          class="w-full border border-line bg-elevated px-3 py-2 font-mono text-[12px] text-txt"
          rows="3"
          readonly
          data-testid="gate-share-url"
          :value="displayUrl || t('pages.gatesInbox.share.urlHiddenUntilCopy')"
          @focus="($event.target as HTMLTextAreaElement).select()"
        />
        <div class="flex flex-wrap gap-2">
          <button
            type="button"
            class="inline-flex min-h-11 items-center gap-1.5 bg-accent px-3 text-xs font-medium text-white hover:bg-accent-2 disabled:opacity-45"
            data-testid="gate-share-copy"
            :disabled="busy"
            @click="copyExisting"
          >
            <Icon name="copy" :size="14" />
            {{ t('common.buttons.copy') }}
          </button>
          <button
            type="button"
            class="inline-flex min-h-11 items-center border border-line px-3 text-xs text-txt2 hover:bg-elevated"
            data-testid="gate-share-regen"
            :disabled="busy"
            @click="confirmKind = 'regen'"
          >
            {{ t('pages.gatesInbox.share.regenerate') }}
          </button>
          <button
            type="button"
            class="inline-flex min-h-11 items-center border border-err/40 px-3 text-xs text-err hover:bg-err/10"
            data-testid="gate-share-revoke"
            :disabled="busy"
            @click="confirmKind = 'revoke'"
          >
            {{ t('pages.gatesInbox.share.revoke') }}
          </button>
        </div>
      </div>

      <div
        v-if="confirmKind"
        class="border border-line bg-elevated px-3 py-3"
        data-testid="gate-share-confirm"
        role="alertdialog"
      >
        <p class="text-[13px] text-txt">
          {{
            confirmKind === 'regen'
              ? t('pages.gatesInbox.share.confirmRegen')
              : t('pages.gatesInbox.share.confirmRevoke')
          }}
        </p>
        <div class="mt-3 flex gap-2">
          <button
            type="button"
            class="min-h-11 bg-accent px-3 text-xs font-medium text-white"
            data-testid="gate-share-confirm-ok"
            :disabled="busy"
            @click="confirmKind === 'regen' ? confirmRegen() : confirmRevoke()"
          >
            {{ t('common.buttons.confirm') }}
          </button>
          <button
            type="button"
            class="min-h-11 border border-line px-3 text-xs text-txt2"
            data-testid="gate-share-confirm-cancel"
            @click="confirmKind = null"
          >
            {{ t('common.buttons.cancel') }}
          </button>
        </div>
      </div>

      <p v-if="errorText" class="text-xs text-err" role="alert" data-testid="gate-share-error">{{ errorText }}</p>
    </div>
  </AppModal>
</template>

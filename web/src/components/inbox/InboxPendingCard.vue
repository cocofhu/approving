<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/ui/Icon.vue'
import { relTime } from '@/lib/format'
import {
  inboxBadgeLabelKey,
  inboxBadgeTone,
  inboxBadgeToneClass,
  inboxIconToneClass,
  inboxSecondaryLine,
} from '@/lib/inboxDisplay'
import { isShareableInboxItem, shareStatusLabel } from '@/lib/gateShareLink'
import type { GateShareInboxStatus, InboxItem } from '@/lib/types'

const props = defineProps<{
  item: InboxItem
  active?: boolean
  disabled?: boolean
  showChevron?: boolean
}>()

const emit = defineEmits<{
  (e: 'select'): void
  (e: 'open-share'): void
}>()

const { t, locale } = useI18n()

const title = computed(() => (props.item.type === 'gate' ? props.item.title : props.item.label))
const secondary = computed(() => inboxSecondaryLine(props.item))
const timeLabel = computed(() =>
  props.item.type === 'gate' ? relTime(props.item.requestedAt) : relTime(props.item.updatedAt),
)
const iconName = computed(() => {
  if (props.item.type === 'gate') return 'gate'
  if (props.item.kind === 'app_preview') return 'monitor'
  return 'chat'
})
const iconClass = computed(() => inboxIconToneClass(inboxBadgeTone(props.item)))
const badgeClass = computed(() => inboxBadgeToneClass(inboxBadgeTone(props.item)))
const badgeText = computed(() => t(inboxBadgeLabelKey(props.item)))
const showShare = computed(() => isShareableInboxItem(props.item))
const itemShareLink = computed((): GateShareInboxStatus | undefined => {
  if (!showShare.value) return undefined
  return 'shareLink' in props.item ? props.item.shareLink : undefined
})
const shareLabel = computed(() => (showShare.value ? shareStatusLabel(itemShareLink.value, t) : ''))
const shareUsed = computed(() => showShare.value && itemShareLink.value?.state === 'used')
</script>

<template>
  <article
    class="flex w-full shrink-0 flex-col border p-3 transition"
    :class="active ? 'border-accent/50 bg-accent-dim/40' : 'border-line bg-surface hover:bg-elevated'"
    data-testid="inbox-item-card"
  >
    <button
      type="button"
      class="flex w-full items-start gap-3 text-left disabled:cursor-not-allowed disabled:opacity-45"
      :disabled="disabled"
      :aria-pressed="active ? 'true' : 'false'"
      @click="emit('select')"
    >
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center"
        :class="iconClass"
      >
        <Icon :name="iconName" :size="18" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="truncate text-sm font-medium text-txt">{{ title }}</div>
        <div class="truncate text-[11px] text-txt3" :title="secondary">{{ secondary }}</div>
        <div class="mt-1 flex items-center gap-1.5">
          <span class="border px-1.5 py-px text-[10px]" :class="badgeClass">{{ badgeText }}</span>
          <span class="text-[10px] text-txt3">{{ locale && timeLabel }}</span>
        </div>
        <div v-if="item.tags?.length" class="mt-1 flex flex-wrap gap-1.5">
          <span v-for="tag in item.tags" :key="tag" class="chip text-txt2">{{ tag }}</span>
        </div>
      </div>
      <Icon v-if="showChevron" name="chevron-right" :size="16" class="mt-2 shrink-0 text-txt3" />
    </button>
    <div
      v-if="showShare"
      class="mt-2 flex flex-wrap items-center gap-2 pl-12"
      data-testid="gate-share-row"
    >
      <span
        role="status"
        class="border border-line bg-elevated px-1.5 py-0.5 text-[10px] text-txt2"
        data-testid="gate-share-status"
      >
        {{ shareLabel }}
      </span>
      <button
        type="button"
        class="inline-flex min-h-11 min-w-[44px] items-center gap-1.5 border border-accent/40 bg-accent/10 px-2.5 text-xs font-medium leading-[1.4] text-accent-2 hover:bg-accent/20 disabled:cursor-not-allowed disabled:opacity-45 md:min-h-6 md:min-w-0 md:gap-1 md:px-1.5 md:py-0.5 md:text-[10px]"
        data-testid="gate-share-copy-btn"
        :disabled="disabled || shareUsed"
        :aria-label="t('pages.gatesInbox.share.copyLinkAria')"
        @click.stop="emit('open-share')"
      >
        <Icon name="copy" :size="12" />
        {{ t('pages.gatesInbox.share.copyLink') }}
      </button>
    </div>
  </article>
</template>

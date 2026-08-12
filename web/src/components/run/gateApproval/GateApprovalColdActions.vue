<script setup lang="ts">
import { useGateApprovalCtx } from './gateApprovalContext'
import Icon from '../../ui/Icon.vue'
import { actionIcon, actionVariant, actionVariantClasses } from './gateApprovalActions'

defineProps<{
  /** mobile fill uses gap-3 + always min-h/flex-1; others vary by isMobile */
  layout: 'mobile' | 'content-fit' | 'desktop'
}>()

const { s } = useGateApprovalCtx()
</script>

<template>
  <div
    class="flex flex-wrap"
    :class="
      layout === 'mobile'
        ? 'gap-3'
        : ['gap-2', s.isMobile ? 'gap-3' : '']
    "
  >
    <button
      v-for="a in s.footerActions"
      :key="a.id"
      class="inline-flex items-center justify-center gap-1.5 rounded-md px-3.5 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50"
      :class="[
        actionVariantClasses(actionVariant(a.id)),
        layout === 'mobile' || s.isMobile ? 'min-h-[44px] flex-1' : 'py-2',
      ]"
      :disabled="s.isActionDisabled(a.id) || s.reactSending"
      :aria-busy="s.actionSubmitting && s.resolved === a.id ? 'true' : undefined"
      :title="s.actionButtonTitle(a.id)"
      data-testid="review-composer-pass"
      @click="s.onSidebarAction(a.id)"
    >
      <Icon
        :name="s.actionSubmitting && s.resolved === a.id ? 'spinner' : actionIcon(a.id)"
        :size="14"
        :class="s.actionSubmitting && s.resolved === a.id ? 'animate-spin' : ''"
        aria-hidden="true"
      />
      {{ s.actionSubmitting && s.resolved === a.id ? s.actionPendingLabel(a.id) : a.label }}
    </button>
  </div>
</template>

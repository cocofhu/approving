<script lang="ts">
export {
  actionIcon,
  actionVariant,
  actionVariantClasses,
  type ActionIconName,
  type ActionVariant,
} from './gateApproval/gateApprovalActions'
</script>

<script setup lang="ts">
import SelectionAddToChat from './SelectionAddToChat.vue'
import GateApprovalTitle from './gateApproval/GateApprovalTitle.vue'
import GateApprovalMobileFill from './gateApproval/GateApprovalMobileFill.vue'
import GateApprovalContentFit from './gateApproval/GateApprovalContentFit.vue'
import GateApprovalDesktopBody from './gateApproval/GateApprovalDesktopBody.vue'
import { useGateApproval } from '@/lib/inbox/useGateApproval'

const props = defineProps<{
  gate: import('@/lib/shared/types').Gate
  run?: import('@/lib/shared/types').Run
  compact?: boolean
  submitError?: string | null
  fillPreview?: boolean
  unifiedPreviewBudget?: boolean
  mobileFillRemaining?: boolean
  shareLink?: import('@/lib/shared/types').GateShareInboxStatus | null
}>()
const emit = defineEmits<{
  (e: 'resolve', action: string, form: Record<string, any>): void
  (e: 'react-revised'): void
  (e: 'open-share'): void
}>()

const {
  useMobileFillRemaining,
  useReviewShellLayout,
  selectionQuoteEnabled,
  gateStageEl,
  onQuoteAdd,
  productEditorRef,
  feedbackChatRef,
  reactAnnotations,
  reactText,
  isEditing,
  applyReviewFrame,
  applyAcpEvents,
  cancelReactRevise,
} = useGateApproval(props, emit)

defineExpose({
  isEditing,
  applyReviewFrame,
  applyAcpEvents,
  cancelReactRevise,
  reactAnnotations,
  reactText,
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <GateApprovalTitle v-if="!compact" />
    <GateApprovalMobileFill v-if="useMobileFillRemaining" />
    <GateApprovalContentFit v-else-if="useReviewShellLayout" />
    <GateApprovalDesktopBody v-else />
    <SelectionAddToChat
      v-if="selectionQuoteEnabled"
      :enabled="selectionQuoteEnabled"
      :root="gateStageEl"
      @add="onQuoteAdd"
    />
  </div>
</template>

<style scoped>
/* Stream four-phase UI styles live in GateReactStreamPanel. */
</style>

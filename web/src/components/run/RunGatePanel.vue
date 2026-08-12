<script setup lang="ts">
/**
 * Run 详情「门禁」面板壳：复用 GateApproval，入口只装配。
 * fill-preview / mobile-fill-remaining 固化在壳内，避免入口重复传参。
 */
import { ref } from 'vue'
import GateApproval from '@/components/run/GateApproval.vue'
import type { Gate, Run } from '@/lib/shared/types'

defineProps<{
  gate: Gate
  run: Run
  submitError?: string | null
}>()

const emit = defineEmits<{
  resolve: [action: string, form: Record<string, any>]
  reactRevised: []
}>()

const gateApprovalRef = ref<{
  applyReviewFrame?: (frame: any) => void
  applyAcpEvents?: (events: any[] | undefined) => boolean | void
} | null>(null)

defineExpose({
  applyReviewFrame: (frame: any) => gateApprovalRef.value?.applyReviewFrame?.(frame),
  applyAcpEvents: (events: any[] | undefined) => gateApprovalRef.value?.applyAcpEvents?.(events),
})
</script>

<template>
  <GateApproval
    ref="gateApprovalRef"
    :gate="gate"
    :run="run"
    :fill-preview="true"
    :mobile-fill-remaining="true"
    :submit-error="submitError"
    @resolve="(action, form) => emit('resolve', action, form)"
    @react-revised="emit('reactRevised')"
  />
</template>

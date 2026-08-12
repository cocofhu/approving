<script setup lang="ts">
/**
 * Run 详情「应用预览」面板壳：复用 AppPreviewPanel。
 */
import AppPreviewPanel from '@/components/run/AppPreviewPanel.vue'

defineProps<{
  runId: string
  nodeId: string
}>()
</script>

<template>
  <div class="flex h-full min-h-0 flex-col p-4">
    <AppPreviewPanel :run-id="runId" :node-id="nodeId" fill />
  </div>
</template>

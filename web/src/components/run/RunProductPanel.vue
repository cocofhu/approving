<script setup lang="ts">
/**
 * Run 详情「产物」面板壳：复用 StructuredProductPanel。
 */
import StructuredProductPanel from '@/components/run/StructuredProductPanel.vue'
import type { NodeRun, Run, WFNode } from '@/lib/shared/types'

defineProps<{
  node: WFNode
  nodeRun: NodeRun
  run: Run
}>()
</script>

<template>
  <StructuredProductPanel :node="node" :node-run="nodeRun" :run="run" />
</template>

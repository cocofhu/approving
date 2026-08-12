<script setup lang="ts">
/**
 * Run 详情「输出」面板壳：复用 NodeOutputPanel。
 */
import NodeOutputPanel from '@/components/run/NodeOutputPanel.vue'
import type { NodeRun, Run, WFNode } from '@/lib/shared/types'

defineProps<{
  node: WFNode
  nodeRun: NodeRun
  run: Run
}>()
</script>

<template>
  <NodeOutputPanel :node="node" :node-run="nodeRun" :run="run" />
</template>

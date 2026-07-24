<script setup lang="ts">
import Icon from '@/components/ui/Icon.vue'
import StructuredProductPanel from '@/components/run/StructuredProductPanel.vue'
import type { ClarifyProductStageKind } from '@/lib/clarifyInboxStage'
import type { NodeRun, Run, WFNode } from '@/lib/types'

defineProps<{
  productNodes: WFNode[]
  selectedProductId: string | null
  stageKind: ClarifyProductStageKind
  selectedNode: WFNode | null
  selectedNodeRun: NodeRun | null
  run: Run | null
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:selectedProductId': [id: string]
  retry: []
}>()
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="clarify-product-stage">
    <div
      v-if="productNodes.length > 1 && stageKind === 'panel'"
      class="flex shrink-0 flex-wrap gap-1 border-b border-line px-3 py-2"
      data-testid="clarify-product-tabs"
    >
      <button
        v-for="n in productNodes"
        :key="n.id"
        type="button"
        class="rounded border px-2 py-0.5 text-[11px] transition"
        :data-testid="'clarify-product-tab-' + n.id"
        :class="
          selectedProductId === n.id
            ? 'border-accent/50 bg-accent-dim/40 text-txt'
            : 'border-line text-txt3 hover:border-line-strong hover:text-txt2'
        "
        @click="emit('update:selectedProductId', n.id)"
      >
        {{ n.label || n.id }}
      </button>
    </div>
    <StructuredProductPanel
      v-if="stageKind === 'panel' && selectedNode && selectedNodeRun && run"
      class="min-h-0 flex-1"
      data-testid="clarify-product-panel"
      :node="selectedNode"
      :node-run="selectedNodeRun"
      :run="run"
      annotatable
    />
    <div
      v-else
      class="flex h-full flex-1 flex-col items-center justify-center p-6 text-center text-[12px] text-txt3"
      :data-testid="'clarify-product-empty-' + stageKind"
    >
      <Icon name="artifact" :size="26" class="mb-2 opacity-40" />
      <div class="mb-1 text-[13px] font-medium text-txt2">
        <template v-if="stageKind === 'pending'">{{ $t('pages.structuredProduct.pendingTitle') }}</template>
        <template v-else-if="stageKind === 'executedEmpty'">{{
          $t('pages.structuredProduct.executedEmptyTitle')
        }}</template>
        <template v-else>{{ $t('pages.structuredProduct.loadFailedTitle') }}</template>
      </div>
      <p class="max-w-[280px] leading-relaxed">
        <template v-if="stageKind === 'pending'">{{ $t('pages.structuredProduct.pending') }}</template>
        <template v-else-if="stageKind === 'executedEmpty'">{{
          $t('pages.structuredProduct.executedEmpty')
        }}</template>
        <template v-else>{{ $t('pages.structuredProduct.loadFailed') }}</template>
      </p>
      <button
        v-if="stageKind === 'loadFailed'"
        type="button"
        class="mt-3.5 inline-flex items-center gap-1.5 border border-accent-2/40 bg-accent-dim px-3 py-1.5 text-[12px] text-accent-2 hover:border-accent-2/70"
        data-testid="clarify-product-retry"
        :disabled="loading"
        @click="emit('retry')"
      >
        <Icon name="refresh" :size="12" />
        {{ loading ? $t('pages.structuredProduct.retrying') : $t('pages.structuredProduct.retry') }}
      </button>
    </div>
  </div>
</template>

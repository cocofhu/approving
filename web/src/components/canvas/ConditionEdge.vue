<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseEdge, EdgeLabelRenderer, getBezierPath, getSmoothStepPath, Position } from '@vue-flow/core'

const { t } = useI18n()
import type { EdgeKind } from '@/lib/shared/types'
import { coordsFinite, fallbackLinePath, roundedPolyline } from '@/lib/run/canvasPath'

const props = defineProps<{
  id: string
  sourceX: number
  sourceY: number
  targetX: number
  targetY: number
  sourcePosition: Position
  targetPosition: Position
  data?: { label?: string; tone?: 'ok' | 'warn' | 'err' | 'default'; kind?: EdgeKind; carry?: string[]; shape?: 'step' }
  markerEnd?: string
  style?: Record<string, any>
  animated?: boolean
}>()

// Normalized to [d, labelX, labelY]; the vue-flow path helpers return a longer
// tuple (with offsets) we don't use, so we keep only the first three fields.
const path = computed<[string, number, number]>(() => {
  const { sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition } = props
  if (!coordsFinite(sourceX, sourceY, targetX, targetY)) {
    return fallbackLinePath(sourceX, sourceY, targetX, targetY)
  }
  const geo = {
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  }
  if (props.data?.shape === 'step') {
    // A forward branch target routes as a plain orthogonal step. A backward /
    // loop-back target (to the left) would otherwise cut straight across the
    // nodes in between — instead route it down through a lane below the nodes,
    // left, then up into the target's input, so it never crosses a node.
    const backward = targetX <= sourceX + 24
    if (backward) {
      const sx = sourceX
      const sy = sourceY
      const tx = targetX
      const ty = targetY
      const m = 26
      const laneY = Math.max(sy, ty) + 96
      const pts: [number, number][] = [
        [sx, sy],
        [sx + m, sy],
        [sx + m, laneY],
        [tx - m, laneY],
        [tx - m, ty],
        [tx, ty],
      ]
      const d = roundedPolyline(pts, 12)
      if (!d) return fallbackLinePath(sx, sy, tx, ty)
      return [d, (sx + tx) / 2, laneY]
    }
    const [d, lx, ly] = getSmoothStepPath({ ...geo, borderRadius: 10 })
    if (!d || /NaN/i.test(d)) return fallbackLinePath(sourceX, sourceY, targetX, targetY)
    return [d, lx, ly]
  }
  const [d, lx, ly] = getBezierPath(geo)
  if (!d || /NaN/i.test(d)) return fallbackLinePath(sourceX, sourceY, targetX, targetY)
  return [d, lx, ly]
})

const labelStyle = computed(() => ({
  transform: `translate(-50%, -50%) translate(${path.value[1]}px,${path.value[2]}px)`,
}))

const toneCls = computed(() => {
  switch (props.data?.tone) {
    case 'ok':
      return 'border-ok/40 text-ok bg-ok/10'
    case 'warn':
      return 'border-warn/40 text-warn bg-warn/10'
    case 'err':
      return 'border-err/40 text-err bg-err/10'
    default:
      return 'border-line text-txt2 bg-elevated'
  }
})

const isRollback = computed(() => props.data?.kind === 'rollback')
const carry = computed(() => props.data?.carry?.length ? props.data!.carry!.join(', ') : '')
</script>

<template>
  <BaseEdge :id="id" :path="path[0]" :marker-end="markerEnd" :style="style" />
  <EdgeLabelRenderer v-if="data?.label || isRollback">
    <div
      class="pointer-events-none absolute flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-medium"
      :class="toneCls"
      :style="labelStyle"
    >
      <span v-if="isRollback" class="text-[9px]">↺</span>
      <span>{{ data?.label || t('common.edgeKinds.rollback.label') }}</span>
      <span v-if="carry" class="rounded bg-warn/15 px-1 text-[9px] text-warn">↩ {{ carry }}</span>
    </div>
  </EdgeLabelRenderer>
</template>

<script setup lang="ts">
import { computed, markRaw, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueFlow, useVueFlow, MarkerType } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import BaseNode from './BaseNode.vue'
import ConditionEdge from './ConditionEdge.vue'
import { getEdgeStroke, type EdgeTone } from './edgeColors'
import { nodeColorHex } from '@/data/nodeRegistry'
import { computeSessionLayout, isInvalidPosition } from '@/lib/canvasLayout'
import {
  flowFingerprint,
  pruneFlowCache,
  reuseFlowElement,
  type FlowNodeCacheEntry,
} from '@/lib/workflowCanvasFlow'
import { theme } from '@/lib/theme'
import type { WFNode, WFEdge, NodeType, NodeRunStatus } from '@/lib/types'

const { t } = useI18n()

const dotColor = computed(() => (theme.value === 'light' ? '#d3d9e4' : '#1c2330'))
const maskColor = computed(() => (theme.value === 'light' ? 'rgba(226,230,238,0.7)' : 'rgba(11,14,20,0.7)'))

const edgeStrokes = computed(() => {
  void theme.value
  return {
    ok: getEdgeStroke('ok'),
    err: getEdgeStroke('err'),
    warn: getEdgeStroke('warn'),
  }
})

const props = defineProps<{
  nodes: WFNode[]
  edges: WFEdge[]
  mode?: 'edit' | 'run'
  statusMap?: Record<string, NodeRunStatus>
  selectedNode?: string | null
  selectedEdge?: string | null
  activePath?: string[] // edge ids to animate (run mode)
}>()

const emit = defineEmits<{
  (e: 'select-node', id: string): void
  (e: 'select-edge', id: string): void
  (e: 'pane-click'): void
  (e: 'drop-node', payload: { type: NodeType; x: number; y: number }): void
  (e: 'connect', payload: { source: string; target: string; sourceHandle?: string | null }): void
  (e: 'move-node', payload: { id: string; x: number; y: number }): void
  (e: 'remove-node', id: string): void
  (e: 'remove-edge', id: string): void
  (e: 'clear-structured-goto', payload: { edgeId: string }): void
}>()

const nodeTypes = { custom: markRaw(BaseNode) } as any
const edgeTypes = { condition: markRaw(ConditionEdge) } as any

const { project, vueFlowRef } = useVueFlow()

const sessionLayout = ref(new Map<string, { x: number; y: number }>())

function layoutStructureKey() {
  return JSON.stringify({
    mode: props.mode,
    nodes: props.nodes.map((n) => ({
      id: n.id,
      type: n.type,
      pos: isInvalidPosition(n.position) ? null : n.position,
      cases: n.type === 'branch' ? (n.config?.cases as { goto?: string }[])?.map((c) => c?.goto) : undefined,
      actions:
        n.type === 'human_gate'
          ? (n.config?.actions as { id?: string; goto?: string }[])?.map((a) => ({ id: a?.id, goto: a?.goto }))
          : undefined,
      exits:
        n.type === 'test' || n.type === 'review'
          ? ['pass', 'fail'].map((k) => (n.config?.exits as Record<string, { goto?: string }>)?.[k]?.goto)
          : undefined,
    })),
    edges: props.edges.map((e) => ({ s: e.source, t: e.target })),
  })
}

watch(
  layoutStructureKey,
  () => {
    if (props.mode === 'run') {
      sessionLayout.value = new Map()
      return
    }
    sessionLayout.value = computeSessionLayout(props.nodes, props.edges)
  },
  { immediate: true },
)

function resolvePosition(n: WFNode) {
  if (props.mode === 'run' || !isInvalidPosition(n.position)) return n.position
  return sessionLayout.value.get(n.id) ?? n.position
}

function branchHandles(n: WFNode) {
  const cases = (n.config?.cases as any[]) || []
  return cases.map((c, i) => {
    const isDefault = c?.when === 'default'
    const when = String(c?.when || '')
    return {
      id: `case-${i}`,
      isDefault,
      label: isDefault
        ? t('pages.workflowEditor.canvas.defaultBranch')
        : when.length > 20
          ? when.slice(0, 19) + '…'
          : when || t('pages.workflowEditor.canvas.branchN', { n: i + 1 }),
    }
  })
}

// human_gate exposes one source handle per action so a line can be dragged from
// each action to its target (like branch cases). Keyed by the stable action id.
function actionHandles(n: WFNode) {
  const actions = (n.config?.actions as any[]) || []
  return actions
    .map((a) => ({ id: String(a?.id ?? ''), label: String(a?.label || a?.id || '') }))
    .filter((a) => a.id)
}

function isPositiveGateAction(id: string): boolean {
  return id === 'approve' || id === 'pass'
}

// human_gate actions render like review/test structured exits (ok/bad chips +
// right-side handles). Routing still comes from config.actions[].goto.
function humanGateExitHandles(n: WFNode) {
  return actionHandles(n).map((a) => ({
    id: a.id,
    label: a.label || a.id,
    tone: (isPositiveGateAction(a.id) ? 'ok' : 'bad') as 'ok' | 'bad',
  }))
}

// test/review expose fixed pass/fail handles for structured gate routing.
function structuredExitHandles(n: WFNode) {
  if (n.type === 'test') {
    return [
      { id: 'pass', label: t('common.structuredExits.testPass'), tone: 'ok' as const },
      { id: 'fail', label: t('common.structuredExits.testFail'), tone: 'bad' as const },
    ]
  }
  if (n.type === 'review') {
    return [
      { id: 'pass', label: t('common.structuredExits.reviewPass'), tone: 'ok' as const },
      { id: 'fail', label: t('common.structuredExits.reviewFail'), tone: 'bad' as const },
    ]
  }
  return []
}

// Cache flow node objects so a selection-only change rebuilds only the
// previously/newly selected nodes — Vue Flow skips reconcile for stable refs.
type FlowNodeObj = {
  id: string
  type: string
  position: { x: number; y: number }
  draggable: boolean
  selected: boolean
  data: Record<string, unknown>
}
const flowNodeCache = new Map<string, FlowNodeCacheEntry<FlowNodeObj>>()

const flowNodes = computed(() => {
  const selectedId = props.selectedNode
  const modeRun = props.mode === 'run'
  const out = props.nodes.map((n) => {
    const position = resolvePosition(n)
    const status = props.statusMap?.[n.id]
    const branches = n.type === 'branch' ? branchHandles(n) : undefined
    const appPreviewReview = n.type === 'app_preview'
    const structuredExits =
      n.type === 'test' || n.type === 'review'
        ? structuredExitHandles(n)
        : n.type === 'human_gate'
          ? humanGateExitHandles(n)
          : undefined
    const selected = selectedId === n.id
    const fingerprint = flowFingerprint({
      type: n.type,
      label: n.label,
      status,
      checkpoint: n.checkpoint,
      position,
      draggable: !modeRun,
      branches,
      appPreviewReview,
      structuredExits,
    })
    return reuseFlowElement(flowNodeCache, n.id, fingerprint, selected, () => ({
      id: n.id,
      type: 'custom',
      position,
      draggable: !modeRun,
      selected,
      data: {
        type: n.type as NodeType,
        label: n.label,
        status,
        checkpoint: n.checkpoint,
        branches,
        appPreviewReview,
        structuredExits,
      },
    }))
  })
  pruneFlowCache(flowNodeCache, props.nodes.map((n) => n.id))
  return out
})

const BRANCH_COLOR = '#E879F9'

function isStructuredGateNode(n: WFNode | undefined): n is WFNode {
  return n?.type === 'test' || n?.type === 'review'
}

function whenHasAction(when: string, action: string): boolean {
  return new RegExp(`action\\s*==\\s*["']${action}["']`).test(when)
}

function whenHasFailAction(when: string): boolean {
  return (
    whenHasAction(when, 'fail') ||
    whenHasAction(when, 'reject') ||
    whenHasAction(when, 'revise')
  )
}

function inferEdgeTone(e: WFEdge, sourceNode: WFNode | undefined): 'ok' | 'err' | 'warn' | 'default' {
  const kind = e.kind || 'success'
  const when = e.when || ''
  const label = e.label || ''

  if (isStructuredGateNode(sourceNode)) {
    if (
      whenHasAction(when, 'fail') ||
      whenHasAction(when, 'reject') ||
      label === t('common.structuredExits.testFail') ||
      label === t('common.structuredExits.reviewFail')
    ) {
      return 'err'
    }
    if (
      whenHasAction(when, 'pass') ||
      label === t('common.structuredExits.testPass') ||
      label === t('common.structuredExits.reviewPass') ||
      label === t('common.edgeLabels.pass')
    ) {
      return 'ok'
    }
  }

  if (kind === 'rollback') return 'warn'
  if (kind === 'failure') return 'err'
  if (
    when.includes('approve') ||
    label === t('common.edgeLabels.approve') ||
    label === t('common.edgeLabels.pass') ||
    label === '批准' ||
    label === '通过'
  ) {
    return 'ok'
  }
  if (
    label === t('common.edgeLabels.revise') ||
    label === t('common.edgeLabels.limit') ||
    label === '退回' ||
    label === '超限'
  ) {
    return 'warn'
  }
  return 'default'
}

// Branch nodes route by their `config.cases[].goto` target (see engine
// execBranch), not by real edges. Derive display-only edges from those rules so
// the routing shows as connection lines on the canvas. They carry a `br:` id
// prefix so change/click handlers can tell them apart from real edges.
const branchEdges = computed(() => {
  const ids = new Set(props.nodes.map((n) => n.id))
  const out: any[] = []
  for (const n of props.nodes) {
    if (n.type !== 'branch') continue
    const cases = (n.config?.cases as any[]) || []
    cases.forEach((c, i) => {
      const target = c?.goto
      if (!target || !ids.has(target)) return
      const isDefault = c?.when === 'default'
      const when = String(c?.when || '')
      const label = isDefault
        ? 'ELSE'
        : when.length > 24
          ? when.slice(0, 23) + '…'
          : when || t('pages.workflowEditor.canvas.branchN', { n: i + 1 })
      out.push({
        id: `br:${n.id}:${i}`,
        source: n.id,
        sourceHandle: `case-${i}`,
        target,
        type: 'condition',
        selectable: false,
        data: { label, tone: isDefault ? 'warn' : 'default', shape: 'step' },
        markerEnd: MarkerType.ArrowClosed,
        style: { stroke: BRANCH_COLOR, strokeWidth: 1.6, strokeDasharray: '5 4' },
      })
    })
  }
  return out
})

// human_gate actions route by their `config.actions[].goto` target (see engine
// ResumeGate), not by real edges — mirror the branch approach and derive
// display-only edges from each action's goto. Prefixed `ga:` so handlers can
// tell them apart from real edges. Stroke tones match review/test exits.
const gateEdges = computed(() => {
  const ids = new Set(props.nodes.map((n) => n.id))
  const strokes = edgeStrokes.value
  const out: any[] = []
  for (const n of props.nodes) {
    if (n.type !== 'human_gate') continue
    const actions = (n.config?.actions as any[]) || []
    for (const a of actions) {
      const aid = String(a?.id ?? '')
      const target = a?.goto
      if (!aid || !target || !ids.has(target)) continue
      const label = String(a?.label || aid)
      const isOk = isPositiveGateAction(aid)
      out.push({
        id: `ga:${n.id}:${aid}`,
        source: n.id,
        sourceHandle: `action-${aid}`,
        target,
        type: 'condition',
        selectable: false,
        data: { label, tone: isOk ? 'ok' : 'err', shape: 'step' },
        markerEnd: MarkerType.ArrowClosed,
        style: { stroke: isOk ? strokes.ok : strokes.err, strokeWidth: 1.6 },
      })
    }
  }
  return out
})

// test/review exits route by config.exits.pass.goto / exits.fail.goto (see engine
// finalizeStructuredGate). Derive display-only edges prefixed `sg:`.
// Edge badge copy is product-canonical (locale-independent); node exit chips keep
// short i18n labels from structuredExitHandles.
const STRUCTURED_EDGE_LABEL_OK = 'Pass · ok'
const STRUCTURED_EDGE_LABEL_ERR = 'Fail / Reject · err'

const structuredGateEdges = computed(() => {
  const ids = new Set(props.nodes.map((n) => n.id))
  const out: any[] = []
  for (const n of props.nodes) {
    if (n.type !== 'test' && n.type !== 'review') continue
    const exits = (n.config?.exits as any) || {}
    const handles = structuredExitHandles(n)
    for (const h of handles) {
      const target = exits[h.id]?.goto
      if (!target || !ids.has(target)) continue
      const isOk = h.id === 'pass' || h.tone === 'ok'
      out.push({
        id: `sg:${n.id}:${h.id}`,
        source: n.id,
        sourceHandle: `action-${h.id}`,
        target,
        type: 'condition',
        selectable: false,
        data: {
          label: isOk ? STRUCTURED_EDGE_LABEL_OK : STRUCTURED_EDGE_LABEL_ERR,
          tone: isOk ? 'ok' : 'err',
          shape: 'step',
        },
        markerEnd: MarkerType.ArrowClosed,
        style: { stroke: isOk ? edgeStrokes.value.ok : edgeStrokes.value.err, strokeWidth: 1.6 },
      })
    }
  }
  return out
})

type FlowEdgeObj = {
  id: string
  source: string
  target: string
  type: string
  animated: boolean
  selected: boolean
  data: Record<string, unknown>
  markerEnd: any
  style: Record<string, unknown>
  sourceHandle?: string
  selectable?: boolean
}
const flowEdgeCache = new Map<string, FlowNodeCacheEntry<FlowEdgeObj>>()

const flowEdges = computed(() => {
  const nodeById = new Map(props.nodes.map((n) => [n.id, n]))
  const selectedEdge = props.selectedEdge
  const strokes = edgeStrokes.value
  const realEdges = props.edges.map((e) => {
    const kind = e.kind || 'success'
    const active = !!props.activePath?.includes(e.id)
    const sourceNode = nodeById.get(e.source)
    const isStructuredGate = isStructuredGateNode(sourceNode)
    const isAppPreview = sourceNode?.type === 'app_preview'
    // Legacy app_preview fail/reject when-edges stay in the graph for silent
    // compat but are not drawn (confirm injects action=pass; fail never routes).
    if (isAppPreview && whenHasFailAction(e.when || '')) {
      return null
    }
    const tone = inferEdgeTone(e, sourceNode)
    // Legacy pass when-edges from app_preview render as the default success exit.
    const treatAsDefaultSuccess =
      isAppPreview && (whenHasAction(e.when || '', 'pass') || whenHasAction(e.when || '', 'approve'))
    const stroke = active
      ? '#7B61FF'
      : isStructuredGate && (tone === 'ok' || tone === 'err') && kind === 'success'
        ? strokes[tone as EdgeTone]
        : treatAsDefaultSuccess
          ? strokes.ok
          : kind === 'failure'
            ? strokes.err
            : kind === 'rollback'
              ? strokes.warn
              : undefined
    const selected = selectedEdge === e.id
    const fingerprint = flowFingerprint({
      source: e.source,
      target: e.target,
      kind,
      label: treatAsDefaultSuccess ? '' : e.label,
      carry: e.carry,
      tone: treatAsDefaultSuccess ? 'ok' : tone,
      active,
      stroke,
      strokeWidth: kind !== 'success' ? 1.6 : undefined,
      dash: kind === 'rollback' ? '6 4' : undefined,
      edgeType: treatAsDefaultSuccess ? 'default' : e.label || kind !== 'success' ? 'condition' : 'default',
      animated: active || kind === 'rollback',
    })
    return reuseFlowElement(flowEdgeCache, e.id, fingerprint, selected, () => ({
      id: e.id,
      source: e.source,
      target: e.target,
      type: treatAsDefaultSuccess ? 'default' : e.label || kind !== 'success' ? 'condition' : 'default',
      animated: !!active || kind === 'rollback',
      selected,
      data: {
        label: treatAsDefaultSuccess ? undefined : e.label,
        tone: treatAsDefaultSuccess ? 'ok' : tone,
        kind,
        carry: e.carry,
      },
      markerEnd: MarkerType.ArrowClosed,
      style: {
        stroke,
        strokeWidth: kind !== 'success' ? 1.6 : undefined,
        strokeDasharray: kind === 'rollback' ? '6 4' : undefined,
      },
    }))
  }).filter(Boolean) as FlowEdgeObj[]
  // Derived edges (branch/gate/structured) are rebuild-cheap and not selection-driven;
  // keep them as-is. Only real edges participate in selected-edge identity reuse.
  const derived = [...branchEdges.value, ...gateEdges.value, ...structuredGateEdges.value]
  pruneFlowCache(
    flowEdgeCache,
    props.edges.map((e) => e.id),
  )
  return [...realEdges, ...derived]
})

function onNodeClick(e: any) {
  emit('select-node', e.node.id)
}
function onEdgeClick(e: any) {
  const id = e.edge?.id as string
  // Derived branch/gate-routing edges aren't real edges; clicking one opens the
  // owning node so the rule/action can be edited in the inspector.
  if ((id?.startsWith('br:') || id?.startsWith('ga:') || id?.startsWith('sg:')) && e.edge?.source) {
    emit('select-node', e.edge.source)
    return
  }
  emit('select-edge', id)
}
function onPaneClick() {
  emit('pane-click')
}
function onConnect(c: any) {
  if (c?.source && c?.target) emit('connect', { source: c.source, target: c.target, sourceHandle: c.sourceHandle })
}
function onNodeDragStop(e: any) {
  const n = e?.node
  if (!n) return
  const { x, y } = n.position
  if (props.mode !== 'run' && isInvalidPosition(props.nodes.find((nd) => nd.id === n.id)?.position)) {
    sessionLayout.value = new Map(sessionLayout.value).set(n.id, { x, y })
  }
  emit('move-node', { id: n.id, x, y })
}
// VueFlow manages its own internal store; mirror its remove changes (e.g. the
// Backspace/Delete key) back to the parent's workflow model so deletions stick.
function onNodesChange(changes: any[]) {
  for (const ch of changes || []) {
    if (ch?.type === 'remove' && ch.id) emit('remove-node', ch.id)
  }
}
function onEdgesChange(changes: any[]) {
  for (const ch of changes || []) {
    if (ch?.type === 'remove' && ch.id) {
      const id = String(ch.id)
      if (id.startsWith('sg:')) {
        emit('clear-structured-goto', { edgeId: id })
        continue
      }
      // Skip derived branch/gate-routing edges: they mirror node config, not real edges.
      if (!id.startsWith('br:') && !id.startsWith('ga:')) emit('remove-edge', ch.id)
    }
  }
}
function onDragOver(ev: DragEvent) {
  ev.preventDefault()
  if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move'
}
function onDrop(ev: DragEvent) {
  const type = ev.dataTransfer?.getData('application/approving-node') as NodeType
  if (!type || !vueFlowRef.value) return
  const bounds = vueFlowRef.value.getBoundingClientRect()
  const pos = project({ x: ev.clientX - bounds.left, y: ev.clientY - bounds.top })
  emit('drop-node', { type, x: pos.x - 88, y: pos.y - 24 })
}

function minimapColor(n: any) {
  return nodeColorHex(n.data.type as NodeType)
}
</script>

<template>
  <div class="h-full w-full" @drop="onDrop" @dragover="onDragOver">
    <VueFlow
      :nodes="flowNodes"
      :edges="flowEdges"
      :node-types="nodeTypes"
      :edge-types="edgeTypes"
      :nodes-draggable="mode !== 'run'"
      :nodes-connectable="mode !== 'run'"
      :elements-selectable="true"
      :min-zoom="0.3"
      :max-zoom="1.75"
      fit-view-on-init
      :default-edge-options="{ type: 'default', markerEnd: MarkerType.ArrowClosed }"
      @node-click="onNodeClick"
      @edge-click="onEdgeClick"
      @pane-click="onPaneClick"
      @connect="onConnect"
      @node-drag-stop="onNodeDragStop"
      @nodes-change="onNodesChange"
      @edges-change="onEdgesChange"
    >
      <Background :gap="18" :size="1.4" :pattern-color="dotColor" />
      <Controls :show-interactive="false" position="bottom-left" />
      <MiniMap pannable :node-color="minimapColor" :mask-color="maskColor" />
    </VueFlow>
  </div>
</template>

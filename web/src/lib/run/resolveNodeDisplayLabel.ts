import { NODE_DEFS as RAW_NODE_DEFS } from '@/data/nodeRegistry'
import type { NodeType, WFNode } from '@/lib/shared/types'

export function resolveNodeDisplayLabel(
  label: string | undefined,
  type: NodeType,
  t: (key: string) => string,
  options?: { nodeId?: string; typeLabel?: string },
): string {
  const rawKey = RAW_NODE_DEFS[type]?.label
  if (label && rawKey && label === rawKey) {
    return t(label)
  }
  if (label) return label
  if (options?.nodeId) return options.nodeId
  if (options?.typeLabel) return options.typeLabel
  if (rawKey) return t(rawKey)
  return type
}

export function resolveNodeDisplayLabelFromNode(
  node: Pick<WFNode, 'id' | 'type' | 'label'>,
  t: (key: string) => string,
  typeLabel?: string,
): string {
  return resolveNodeDisplayLabel(node.label, node.type, t, {
    nodeId: node.id,
    typeLabel,
  })
}

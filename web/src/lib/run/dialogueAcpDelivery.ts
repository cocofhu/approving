import type { AcpEvent } from '../shared/types'

export type DialogueAcpApplyResult = 'applied' | 'buffer'

/**
 * Deliver cumulative ACP to clarify and/or gate surfaces.
 * `apply*` must return false when the inner chat/gate is not ready
 * (e.g. ReviewComposer wrapping ClarifyChat before mount); undefined/true = ok.
 */
export function deliverOrBufferDialogueAcp(opts: {
  forClarify: boolean
  forGate: boolean
  events: AcpEvent[]
  nodeId: string
  applyClarify?: (events: AcpEvent[], nodeId: string) => boolean | void
  applyGate?: (events: AcpEvent[]) => boolean | void
}): DialogueAcpApplyResult {
  const { forClarify, forGate, events, nodeId } = opts
  if (!forClarify && !forGate) return 'buffer'
  let applied = false
  if (forClarify && opts.applyClarify) {
    if (opts.applyClarify(events, nodeId) !== false) applied = true
  }
  if (forGate && opts.applyGate) {
    if (opts.applyGate(events) !== false) applied = true
  }
  return applied ? 'applied' : 'buffer'
}

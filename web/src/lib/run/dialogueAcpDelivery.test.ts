import { describe, expect, it, vi } from 'vitest'
import { deliverOrBufferDialogueAcp } from './dialogueAcpDelivery'
import type { AcpEvent } from '../shared/types'

const sample: AcpEvent[] = [
  { kind: 'thought', text: 't1', t: 1 },
  { kind: 'message', text: 'm1', t: 2 },
]

describe('deliverOrBufferDialogueAcp', () => {
  it('buffers when neither surface matches', () => {
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: false,
        forGate: false,
        events: sample,
        nodeId: 'n1',
      }),
    ).toBe('buffer')
  })

  it('buffers when ReviewComposer exposes apply but inner chat not ready (returns false)', () => {
    const applyClarify = vi.fn(() => false)
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: true,
        forGate: false,
        events: sample,
        nodeId: 'clarify-1',
        applyClarify,
      }),
    ).toBe('buffer')
    expect(applyClarify).toHaveBeenCalledWith(sample, 'clarify-1')
  })

  it('marks applied when clarify apply succeeds (void/true)', () => {
    const applyClarify = vi.fn(() => undefined)
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: true,
        forGate: false,
        events: sample,
        nodeId: 'clarify-1',
        applyClarify,
      }),
    ).toBe('applied')
  })

  it('applies gate and buffers clarify when only gate ready', () => {
    const applyGate = vi.fn(() => true)
    const applyClarify = vi.fn(() => false)
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: true,
        forGate: true,
        events: sample,
        nodeId: 'producer',
        applyClarify,
        applyGate,
      }),
    ).toBe('applied')
    expect(applyGate).toHaveBeenCalled()
  })

  it('buffers when both apply helpers report not ready', () => {
    expect(
      deliverOrBufferDialogueAcp({
        forClarify: true,
        forGate: true,
        events: sample,
        nodeId: 'producer',
        applyClarify: () => false,
        applyGate: () => false,
      }),
    ).toBe('buffer')
  })
})

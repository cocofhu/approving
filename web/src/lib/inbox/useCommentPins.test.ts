import { describe, it, expect, beforeEach } from 'vitest'
import {
  saveCommentPin,
  deleteCommentPin,
  listCommentPins,
  getCommentPinRound,
  markCommentPinsCommitted,
  buildAnnotationArtifactPayload,
  formatAnnotationArtifactPreview,
  resetCommentPinsForTests,
} from './useCommentPins'

describe('useCommentPins', () => {
  const runId = 'run-t'
  const nodeId = 'gate-t'
  const iter = 3

  beforeEach(() => {
    resetCommentPinsForTests()
  })

  it('assigns monotonic seq and does not recycle on delete', () => {
    const a = saveCommentPin(runId, nodeId, iter, {
      selector: 'div.a',
      comment: 'one',
    })
    const b = saveCommentPin(runId, nodeId, iter, {
      selector: 'div.a',
      comment: 'two',
    })
    expect(a?.seq).toBe(1)
    expect(b?.seq).toBe(2)
    deleteCommentPin(runId, nodeId, iter, a!.id)
    const c = saveCommentPin(runId, nodeId, iter, {
      selector: 'div.b',
      comment: 'three',
    })
    expect(c?.seq).toBe(3)
    const list = listCommentPins(runId, nodeId, iter)
    expect(list.map((p) => p.seq)).toEqual([2, 3])
  })

  it('allows empty screenshot (MISSING) while keeping selector', () => {
    const pin = saveCommentPin(runId, nodeId, iter + 1, {
      selector: 'h1',
      comment: '无图也可',
    })
    expect(pin?.screenshot).toBe('MISSING')
    expect(pin?.selector).toBe('h1')
    expect(pin?.imageDataUrl).toBeUndefined()
  })

  it('cancels committed state on mutate; mark committed restores', () => {
    saveCommentPin(runId, nodeId, iter + 2, { selector: 'x', comment: 'a' })
    markCommentPinsCommitted(runId, nodeId, iter + 2)
    expect(getCommentPinRound(runId, nodeId, iter + 2).artifactCommitted).toBe(true)
    saveCommentPin(runId, nodeId, iter + 2, { selector: 'y', comment: 'b' })
    expect(getCommentPinRound(runId, nodeId, iter + 2).artifactCommitted).toBe(false)
  })

  it('buildAnnotationArtifactPayload includes hardScope and MISSING', () => {
    const pin = saveCommentPin(runId, nodeId, iter + 3, {
      selector: '.cta',
      comment: '按钮文案',
    })!
    const payload = buildAnnotationArtifactPayload([pin])
    expect(payload.hardScope).toContain('仅改标中区域')
    expect(payload.route).toBe('artifact_only')
    expect(payload.annotations[0].screenshot).toBe('MISSING')
    expect(formatAnnotationArtifactPreview([pin])).toContain('MISSING')
  })

  it('isolates rounds by iteration', () => {
    saveCommentPin(runId, nodeId, 1, { selector: 'a', comment: 'r1' })
    expect(listCommentPins(runId, nodeId, 2)).toEqual([])
    expect(listCommentPins(runId, nodeId, 1)).toHaveLength(1)
  })
})

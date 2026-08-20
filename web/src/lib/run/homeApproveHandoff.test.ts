import { describe, expect, it } from 'vitest'
import {
  consumeHomeApproveHandoff,
  peekHomeApproveHandoff,
  setHomeApproveHandoff,
  takeHomeApproveHandoff,
  updateHomeApproveHandoffNode,
} from './homeApproveHandoff'

describe('homeApproveHandoff', () => {
  it('stores then consumes once', () => {
    setHomeApproveHandoff({
      runId: 'run-1',
      nodeId: 'ap',
      text: '做登录',
      images: [{ mimeType: 'image/png', data: 'abc', name: 'a.png' }],
    })
    expect(peekHomeApproveHandoff()?.text).toBe('做登录')
    const got = takeHomeApproveHandoff()
    expect(got?.runId).toBe('run-1')
    expect(got?.images).toHaveLength(1)
    expect(takeHomeApproveHandoff()).toBeNull()
  })

  it('consumeMatching leaves a mismatched card in the slot', () => {
    setHomeApproveHandoff({
      runId: 'run-1',
      nodeId: 'ap',
      text: '做登录',
      images: [],
    })
    expect(consumeHomeApproveHandoff('run-other', 'ap')).toBeNull()
    expect(peekHomeApproveHandoff()?.runId).toBe('run-1')
    expect(consumeHomeApproveHandoff('run-1', 'ap')?.text).toBe('做登录')
    expect(peekHomeApproveHandoff()).toBeNull()
  })

  it('delivers attachments on the same run when node id is still empty', () => {
    setHomeApproveHandoff({
      runId: 'run-1',
      nodeId: '',
      text: '附图',
      images: [{ mimeType: 'application/pdf', data: 'QQ==', name: 'a.pdf' }],
    })
    const got = consumeHomeApproveHandoff('run-1', 'ap')
    expect(got?.images).toEqual([{ mimeType: 'application/pdf', data: 'QQ==', name: 'a.pdf' }])
    expect(got?.nodeId).toBe('ap')
  })

  it('updates the parked node id only while the slot is still held', () => {
    setHomeApproveHandoff({ runId: 'run-1', nodeId: '', text: '附图', images: [] })
    expect(updateHomeApproveHandoffNode('run-other', 'ap')).toBe(false)
    expect(updateHomeApproveHandoffNode('run-1', 'ap')).toBe(true)
    expect(peekHomeApproveHandoff()?.nodeId).toBe('ap')
    takeHomeApproveHandoff()
    expect(updateHomeApproveHandoffNode('run-1', 'ap')).toBe(false)
  })
})

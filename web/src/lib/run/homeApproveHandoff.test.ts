import { describe, expect, it } from 'vitest'
import {
  consumeHomeApproveHandoff,
  peekHomeApproveHandoff,
  setHomeApproveHandoff,
  takeHomeApproveHandoff,
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
})

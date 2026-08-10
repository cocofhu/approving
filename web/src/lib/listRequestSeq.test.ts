import { describe, expect, it } from 'vitest'
import { createListRequestSeq, httpStatusOf } from './listRequestSeq'

describe('createListRequestSeq', () => {
  it('beginListRequest increments and isCurrentSeq tracks the latest intent', () => {
    const seq = createListRequestSeq()
    const first = seq.beginListRequest()
    const second = seq.beginListRequest()
    expect(first).toBe(1)
    expect(second).toBe(2)
    expect(seq.isCurrentSeq(first)).toBe(false)
    expect(seq.isCurrentSeq(second)).toBe(true)
    expect(seq.currentSeq()).toBe(2)
  })

  it('independent instances do not share counters', () => {
    const a = createListRequestSeq()
    const b = createListRequestSeq()
    expect(a.beginListRequest()).toBe(1)
    expect(b.beginListRequest()).toBe(1)
    expect(a.isCurrentSeq(1)).toBe(true)
    expect(b.isCurrentSeq(1)).toBe(true)
  })
})

describe('httpStatusOf', () => {
  it('reads numeric status from thrown API errors', () => {
    expect(httpStatusOf(Object.assign(new Error('denied'), { status: 403 }))).toBe(403)
    expect(httpStatusOf(new Error('plain'))).toBeUndefined()
    expect(httpStatusOf(null)).toBeUndefined()
  })
})

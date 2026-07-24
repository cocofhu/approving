import { describe, expect, it } from 'vitest'
import {
  fmtTokenCount,
  fmtTokenRate,
  summarizeTimelineUsage,
  tokenUsageTotal,
} from './tokenUsage'

describe('tokenUsage', () => {
  it('sums four components', () => {
    expect(
      tokenUsageTotal({
        inputTokens: 10,
        outputTokens: 5,
        cacheReadTokens: 2,
        cacheWriteTokens: 1,
      }),
    ).toBe(18)
  })

  it('formats counts and rates like Demo', () => {
    expect(fmtTokenCount(1234)).toBe('1,234')
    expect(fmtTokenRate(100, 10)).toBe('10.0')
    expect(fmtTokenRate(5, 10)).toBe('0.50')
    expect(fmtTokenRate(100, 0)).toBeNull()
  })

  it('summarizes: no usage → —; reported 0 → 0; partial sum', () => {
    expect(summarizeTimelineUsage([{ usage: null }, {}], 60)).toEqual({
      totalTokens: null,
      tokenRate: null,
    })
    expect(
      summarizeTimelineUsage(
        [
          {
            usage: {
              inputTokens: 0,
              outputTokens: 0,
              cacheReadTokens: 0,
              cacheWriteTokens: 0,
            },
          },
        ],
        10,
      ),
    ).toEqual({ totalTokens: 0, tokenRate: '0.00' })
    expect(
      summarizeTimelineUsage(
        [
          { usage: null },
          {
            usage: {
              inputTokens: 100,
              outputTokens: 50,
              cacheReadTokens: 10,
              cacheWriteTokens: 0,
            },
          },
          {
            usage: {
              inputTokens: 20,
              outputTokens: 0,
              cacheReadTokens: 0,
              cacheWriteTokens: 5,
            },
          },
        ],
        35,
      ),
    ).toEqual({ totalTokens: 185, tokenRate: '5.29' })
  })
})

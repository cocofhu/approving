import { describe, expect, it } from 'vitest'
import {
  fmtCompactTokenCount,
  fmtTokenCount,
  fmtTokenRate,
  mergeTokenUsage,
  summarizeMultiRunUsage,
  summarizeTimelineUsage,
  sumTotalTokens,
  tokenUsageTotal,
  totalTokensOrNull,
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

  it('formats project totals with Demo K/M compact rules', () => {
    expect(fmtCompactTokenCount(null)).toBe('—')
    expect(fmtCompactTokenCount(undefined)).toBe('—')
    expect(fmtCompactTokenCount(0)).toBe('0')
    expect(fmtCompactTokenCount(42)).toBe('42')
    expect(fmtCompactTokenCount(999)).toBe('999')
    expect(fmtCompactTokenCount(1000)).toBe('1K')
    expect(fmtCompactTokenCount(128400)).toBe('128.4K')
    expect(fmtCompactTokenCount(1_020_000)).toBe('1.02M')
    expect(fmtCompactTokenCount(1_000_000)).toBe('1M')
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

  it('merges usage components and skips unreported', () => {
    expect(mergeTokenUsage(null, undefined)).toBeNull()
    expect(
      mergeTokenUsage(
        { inputTokens: 10, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 },
        null,
        { inputTokens: 5, outputTokens: 3, cacheReadTokens: 1, cacheWriteTokens: 2 },
      ),
    ).toEqual({
      inputTokens: 15,
      outputTokens: 3,
      cacheReadTokens: 1,
      cacheWriteTokens: 2,
    })
    expect(totalTokensOrNull(null)).toBeNull()
    expect(
      totalTokensOrNull({
        inputTokens: 0,
        outputTokens: 0,
        cacheReadTokens: 0,
        cacheWriteTokens: 0,
      }),
    ).toBe(0)
    expect(sumTotalTokens(null, 10, null, 5)).toBe(15)
    expect(sumTotalTokens(null, undefined)).toBeNull()
  })

  it('summarizes multi-run: ignores runs without usage for Σ/avg denom', () => {
    expect(summarizeMultiRunUsage([null, null], 100)).toEqual({
      totalTokens: null,
      usageRunCount: 0,
      avgTokens: null,
      tokenRate: null,
    })
    expect(summarizeMultiRunUsage([100, null, 50], 200)).toEqual({
      totalTokens: 150,
      usageRunCount: 2,
      avgTokens: 75,
      tokenRate: '0.75',
    })
    expect(summarizeMultiRunUsage([0], 0)).toEqual({
      totalTokens: 0,
      usageRunCount: 1,
      avgTokens: 0,
      tokenRate: null,
    })
  })
})

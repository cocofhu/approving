import { beforeAll, afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatTrigger, fmtCompactDuration, fmtDuration, relTime, truncateText } from './format'
import { i18n } from './i18n'
import { loadLocaleMessages } from './loadLocaleMessages'

beforeAll(async () => {
  const [zh, en] = await Promise.all([
    loadLocaleMessages('zh-CN'),
    loadLocaleMessages('en'),
  ])
  i18n.global.setLocaleMessage('zh-CN', zh)
  i18n.global.setLocaleMessage('en', en)
})

describe('fmtCompactDuration', () => {
  it('aligns Demo thresholds: h / m / 0s', () => {
    expect(fmtCompactDuration(3703)).toBe('1.03h')
    expect(fmtCompactDuration(3458)).toBe('57.6m')
    expect(fmtCompactDuration(245)).toBe('4.1m')
    expect(fmtCompactDuration(0)).toBe('0s')
  })

  it('handles short durations under one minute as fractional minutes', () => {
    expect(fmtCompactDuration(30)).toBe('0.5m')
    expect(fmtCompactDuration(6)).toBe('0.1m')
  })

  it('keeps clock fmtDuration for tip exactness', () => {
    expect(fmtDuration(3703)).toBe('01:01:43')
    expect(fmtDuration(3458)).toBe('57:38')
    expect(fmtDuration(245)).toBe('04:05')
    expect(fmtDuration(0)).toBe('00:00')
  })
})

describe('relTime', () => {
  const now = new Date('2026-07-04T12:00:00Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(now)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function isoSecondsAgo(seconds: number): string {
    return new Date(now.getTime() - seconds * 1000).toISOString()
  }

  it('returns empty string for empty iso', () => {
    expect(relTime('')).toBe('')
  })

  it('formats zh-CN thresholds', () => {
    i18n.global.locale.value = 'zh-CN'
    expect(relTime(isoSecondsAgo(30))).toBe('刚刚')
    expect(relTime(isoSecondsAgo(300))).toBe('5 分钟前')
    expect(relTime(isoSecondsAgo(7200))).toBe('2 小时前')
    expect(relTime(isoSecondsAgo(172800))).toBe('2 天前')
  })

  it('formats en thresholds', () => {
    i18n.global.locale.value = 'en'
    expect(relTime(isoSecondsAgo(30))).toBe('Just now')
    expect(relTime(isoSecondsAgo(300))).toBe('5 min ago')
    expect(relTime(isoSecondsAgo(7200))).toBe('2 h ago')
    expect(relTime(isoSecondsAgo(172800))).toBe('2 d ago')
  })
})

describe('formatTrigger', () => {
  it('maps whitelist codes to Demo-aligned labels (zh-CN)', () => {
    i18n.global.locale.value = 'zh-CN'
    expect(formatTrigger('manual')).toBe('手动')
    expect(formatTrigger('api')).toBe('API')
    expect(formatTrigger('pm_mcp')).toBe('项目管理 MCP')
  })

  it('maps whitelist codes to Demo-aligned labels (en)', () => {
    i18n.global.locale.value = 'en'
    expect(formatTrigger('manual')).toBe('Manual')
    expect(formatTrigger('api')).toBe('API')
    expect(formatTrigger('pm_mcp')).toBe('Project Management MCP')
  })

  it('maps historical aliases to the same standard labels', () => {
    i18n.global.locale.value = 'zh-CN'
    expect(formatTrigger('手动触发')).toBe('手动')
    expect(formatTrigger('API 触发')).toBe('API')
    // Historical storage alias "PM MCP" still maps; display uses product naming
    expect(formatTrigger('PM MCP')).toBe('项目管理 MCP')

    i18n.global.locale.value = 'en'
    expect(formatTrigger('手动触发')).toBe('Manual')
    expect(formatTrigger('API 触发')).toBe('API')
    expect(formatTrigger('PM MCP')).toBe('Project Management MCP')
  })

  it('returns unmapped free-form values unchanged', () => {
    i18n.global.locale.value = 'en'
    expect(formatTrigger('channel')).toBe('channel')
    expect(formatTrigger('qq:cron-timezone-bug')).toBe('qq:cron-timezone-bug')
    expect(formatTrigger('cron-nightly')).toBe('cron-nightly')
  })
})

describe('truncateText', () => {
  it('never ends a label in the middle of a word', () => {
    const title = '调研 Approving 最近关于快模型和 worker 架构的精简空间'
    expect(truncateText(title, 24)).toBe('调研 Approving 最近关于快模型和…')
    expect(truncateText(title, 30)).toBe('调研 Approving 最近关于快模型和 worker 架…')
  })

  it('cuts Chinese at any character, since it has no word spacing', () => {
    expect(truncateText('两个检查还在跑其余全部通过', 8)).toBe('两个检查还在跑其…')
  })

  it('leaves text within budget untouched', () => {
    expect(truncateText('登录页性能优化', 60)).toBe('登录页性能优化')
  })

  it('counts code points, so an emoji is not split into squares', () => {
    expect(truncateText('🎉🎉🎉🎉', 2)).toBe('🎉🎉…')
  })
})

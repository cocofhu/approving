import { beforeAll, afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatTrigger, relTime } from './format'
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
    expect(formatTrigger('pm_mcp')).toBe('PM MCP')
  })

  it('maps whitelist codes to Demo-aligned labels (en)', () => {
    i18n.global.locale.value = 'en'
    expect(formatTrigger('manual')).toBe('Manual')
    expect(formatTrigger('api')).toBe('API')
    expect(formatTrigger('pm_mcp')).toBe('PM MCP')
  })

  it('maps historical aliases to the same standard labels', () => {
    i18n.global.locale.value = 'zh-CN'
    expect(formatTrigger('手动触发')).toBe('手动')
    expect(formatTrigger('API 触发')).toBe('API')
    expect(formatTrigger('PM MCP')).toBe('PM MCP')

    i18n.global.locale.value = 'en'
    expect(formatTrigger('手动触发')).toBe('Manual')
    expect(formatTrigger('API 触发')).toBe('API')
    expect(formatTrigger('PM MCP')).toBe('PM MCP')
  })

  it('returns unmapped free-form values unchanged', () => {
    i18n.global.locale.value = 'en'
    expect(formatTrigger('channel')).toBe('channel')
    expect(formatTrigger('qq:cron-timezone-bug')).toBe('qq:cron-timezone-bug')
    expect(formatTrigger('cron-nightly')).toBe('cron-nightly')
  })
})

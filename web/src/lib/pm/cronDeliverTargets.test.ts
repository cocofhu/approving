// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { deriveRecentPushTargets, pushTargetPrimaryLabel } from './cronDeliverTargets'

describe('deriveRecentPushTargets', () => {
  it('filters qq: threads, strips prefix, and drops non-channel threads', () => {
    const out = deriveRecentPushTargets([
      { userId: 'qq:guild:111', title: 'A', updatedAt: '2026-01-02T00:00:00Z' },
      { userId: 'user-web-1', title: 'web', updatedAt: '2026-01-03T00:00:00Z' },
      { userId: 'cron:job-1', title: 'cron', updatedAt: '2026-01-04T00:00:00Z' },
      { userId: 'qq:group:222', title: 'B', updatedAt: '2026-01-01T00:00:00Z' },
    ])
    expect(out.map((o) => o.value)).toEqual(['guild:111', 'group:222'])
  })

  it('dedupes by value keeping the newest updatedAt', () => {
    const out = deriveRecentPushTargets([
      { userId: 'qq:guild:111', title: 'old', updatedAt: '2026-01-01T00:00:00Z' },
      { userId: 'qq:guild:111', title: 'new', updatedAt: '2026-01-05T00:00:00Z' },
      { userId: 'qq:guild:111', title: 'mid', updatedAt: '2026-01-03T00:00:00Z' },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ value: 'guild:111', title: 'new' })
  })

  it('sorts by updatedAt descending and caps at 10', () => {
    const threads = Array.from({ length: 12 }, (_, i) => ({
      userId: `qq:guild:${i}`,
      title: `t${i}`,
      updatedAt: `2026-01-${String(i + 1).padStart(2, '0')}T00:00:00Z`,
    }))
    const out = deriveRecentPushTargets(threads)
    expect(out).toHaveLength(10)
    expect(out[0].value).toBe('guild:11')
    expect(out[9].value).toBe('guild:2')
    expect(out.every((o) => !o.value.startsWith('qq:'))).toBe(true)
  })

  it('preserves empty titles for UI fallback', () => {
    const out = deriveRecentPushTargets([
      { userId: 'qq:c2c:abc', title: '', updatedAt: '2026-01-01T00:00:00Z' },
    ])
    expect(out[0]).toMatchObject({ value: 'c2c:abc', title: '' })
    expect(pushTargetPrimaryLabel(out[0])).toBe('c2c:abc')
  })

  it('skips qq: with empty remainder', () => {
    expect(deriveRecentPushTargets([{ userId: 'qq:', title: 'x', updatedAt: '2026-01-01T00:00:00Z' }])).toEqual([])
  })

  it('drops invalid scene or empty conversationId', () => {
    const out = deriveRecentPushTargets([
      { userId: 'qq:channel:999', title: 'bad scene', updatedAt: '2026-01-03T00:00:00Z' },
      { userId: 'qq:guild:', title: 'empty id', updatedAt: '2026-01-02T00:00:00Z' },
      { userId: 'qq:group:ok', title: 'keep', updatedAt: '2026-01-01T00:00:00Z' },
    ])
    expect(out.map((o) => o.value)).toEqual(['group:ok'])
  })

  it('keeps wecom: threads and marks unspoken', () => {
    const out = deriveRecentPushTargets([
      { userId: 'wecom:c2c:zhangsan', title: '企微', updatedAt: '2026-01-04T00:00:00Z' },
      { userId: 'wecom:c2c:silent', title: '', updatedAt: '2026-01-03T00:00:00Z', unspoken: true },
      { userId: 'qq:c2c:u1', title: 'QQ', updatedAt: '2026-01-02T00:00:00Z' },
    ])
    expect(out.map((o) => o.value)).toEqual(['c2c:zhangsan', 'c2c:silent', 'c2c:u1'])
    expect(out[0].channelType).toBe('wecom')
    expect(pushTargetPrimaryLabel(out[1])).toContain('未发言')
  })
})

describe('pushTargetPrimaryLabel', () => {
  it('uses title when present, otherwise value', () => {
    expect(pushTargetPrimaryLabel({ value: 'guild:1', title: '周会' })).toBe('周会')
    expect(pushTargetPrimaryLabel({ value: 'guild:1', title: '  ' })).toBe('guild:1')
  })
})

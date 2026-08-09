import { describe, expect, it } from 'vitest'
import {
  maskShareUrl,
  parseShareTokenFromHash,
  formatRemainingSec,
  shareStatusLabel,
  canCreateGateShare,
  isGateShareActive,
} from './gateShareLink'

const t = (key: string, values?: Record<string, unknown>) => {
  if (key.endsWith('remainingDays')) return `${values?.n}d`
  if (key.endsWith('remainingHours')) return `${values?.n}h`
  if (key.endsWith('remainingMinutes')) return `${values?.n}m`
  if (key.endsWith('expired')) return 'expired'
  if (key.endsWith('stateNone')) return 'none'
  if (key.endsWith('stateUsed')) return 'used'
  if (key.endsWith('stateRevoked')) return 'revoked'
  if (key.endsWith('stateExpired')) return 'expired'
  if (key.endsWith('stateActive')) return `active ${values?.remaining}`
  return key
}

describe('gateShareLink helpers', () => {
  it('masks fragment token and parses #t= only', () => {
    const token = 'a'.repeat(64)
    const url = `https://app.example/public/gate-approvals#t=${token}`
    expect(maskShareUrl(url)).toBe('https://app.example/public/gate-approvals#t=••••••••')
    expect(maskShareUrl(url)).not.toContain(token)
    expect(parseShareTokenFromHash(`#t=${token}`)).toBe(token)
    expect(parseShareTokenFromHash('?token=abc')).toBe('')
    expect(parseShareTokenFromHash('')).toBe('')
  })

  it('formats remaining time and status chips', () => {
    expect(formatRemainingSec(0, t)).toBe('expired')
    expect(formatRemainingSec(90, t)).toBe('1m')
    expect(formatRemainingSec(3700, t)).toBe('1h')
    expect(formatRemainingSec(2 * 86400, t)).toBe('2d')
    expect(shareStatusLabel({ state: 'none' }, t)).toBe('none')
    expect(shareStatusLabel({ state: 'active', remainingSec: 3600 }, t)).toBe('active 1h')
    expect(shareStatusLabel({ state: 'used' }, t)).toBe('used')
  })

  it('blocks recreate after used; allows after revoke/expired', () => {
    expect(canCreateGateShare({ state: 'used', canCreate: false })).toBe(false)
    expect(canCreateGateShare({ state: 'revoked', canCreate: true })).toBe(true)
    expect(canCreateGateShare({ state: 'expired', canCreate: true })).toBe(true)
    expect(isGateShareActive({ state: 'active' })).toBe(true)
  })
})

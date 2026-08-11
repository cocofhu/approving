import { describe, expect, it } from 'vitest'
import {
  getRegionPolicy,
  isManagedRegionKey,
  normalizeRegions,
  regionSummary,
  setRegion,
  switchBackendRegions,
} from './regionPolicy'

describe('region policy', () => {
  it('defines the four canonical mappings and international defaults', () => {
    expect(getRegionPolicy('codebuddy')).toMatchObject({
      regionEnvKey: 'APPROVING_CODEBUDDY_REGION',
      defaultRegion: 'public',
    })
    expect(getRegionPolicy('codebuddy')?.options.map((item) => item.id)).toEqual([
      'internal',
      'public',
    ])
    expect(getRegionPolicy('trae')).toMatchObject({
      regionEnvKey: 'APPROVING_TRAE_REGION',
      defaultRegion: 'intl',
    })
    expect(getRegionPolicy('trae')?.options.map((item) => item.id)).toEqual(['cn', 'intl'])
    expect(getRegionPolicy('cursor')).toBeUndefined()
    expect(getRegionPolicy('claude_code')).toBeUndefined()
  })

  it('switches backend by clearing all managed keys and writing the target default', () => {
    const env = {
      KEEP: 'yes',
      APPROVING_CODEBUDDY_REGION: 'internal',
      APPROVING_TRAE_REGION: 'cn',
    }
    expect(switchBackendRegions(env, 'trae')).toEqual({
      KEEP: 'yes',
      APPROVING_TRAE_REGION: 'intl',
    })
    expect(switchBackendRegions(env, 'cursor')).toEqual({ KEEP: 'yes' })
  })

  it('strict mode removes conflicts and replaces missing or unknown values', () => {
    expect(
      normalizeRegions(
        { APPROVING_CODEBUDDY_REGION: 'ioa', APPROVING_TRAE_REGION: 'cn' },
        'codebuddy',
        'strict',
      ),
    ).toEqual({
      env: { APPROVING_CODEBUDDY_REGION: 'public' },
      region: 'public',
      special: false,
    })
  })

  it('preserve-special keeps non-empty CodeBuddy legacy and unknown values', () => {
    for (const value of ['ioa', 'staging', 'private-edge']) {
      expect(
        normalizeRegions({ APPROVING_CODEBUDDY_REGION: value }, 'codebuddy', 'preserve-special'),
      ).toEqual({
        env: { APPROVING_CODEBUDDY_REGION: value },
        region: value,
        special: true,
      })
    }
    expect(
      normalizeRegions({}, 'codebuddy', 'preserve-special').env.APPROVING_CODEBUDDY_REGION,
    ).toBe('public')
  })

  it('active selection overwrites a special value with a canonical site', () => {
    const next = setRegion({ APPROVING_CODEBUDDY_REGION: 'ioa' }, 'codebuddy', 'internal')
    expect(regionSummary(next, 'codebuddy', 'preserve-special')).toMatchObject({
      region: 'internal',
      site: 'domestic',
      special: false,
    })
  })

  it('recognizes managed keys only', () => {
    expect(isManagedRegionKey(' APPROVING_TRAE_REGION ')).toBe(true)
    expect(isManagedRegionKey('OTHER')).toBe(false)
  })
})

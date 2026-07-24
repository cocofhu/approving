import { describe, expect, it } from 'vitest'
import { sidebarNavGroups } from './sidebarNav'

describe('sidebarNav', () => {
  it('exposes dashboard and config groups', () => {
    expect(sidebarNavGroups.length).toBeGreaterThanOrEqual(2)
    expect(sidebarNavGroups[0].items.some((i) => i.to === '/dashboard')).toBe(true)
    expect(sidebarNavGroups[1].titleKey).toBe('nav.groupConfig')
    expect(sidebarNavGroups[1].items.some((i) => i.to === '/settings')).toBe(true)
  })

  it('does not expose a global /board entry', () => {
    const allTos = sidebarNavGroups.flatMap((g) => g.items.map((i) => i.to))
    expect(allTos).not.toContain('/board')
    expect(allTos).toContain('/projects')
  })
})

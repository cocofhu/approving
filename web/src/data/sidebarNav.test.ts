import { describe, expect, it } from 'vitest'
import { sidebarNavGroups } from './sidebarNav'

describe('sidebarNav (plan g1.1)', () => {
  it('workspace primary is exactly home / gates / notifications / settings', () => {
    expect(sidebarNavGroups).toHaveLength(1)
    expect(sidebarNavGroups[0].titleKey).toBeUndefined()
    expect(sidebarNavGroups[0].items.map((i) => i.to)).toEqual([
      '/dashboard',
      '/gates',
      '/notifications',
      '/settings',
    ])
  })

  it('does not expose collapsed pages or config group as workspace primary', () => {
    const allTos = sidebarNavGroups.flatMap((g) => g.items.map((i) => i.to))
    expect(allTos).not.toContain('/stats')
    expect(allTos).not.toContain('/projects')
    expect(allTos).not.toContain('/runs')
    expect(allTos).not.toContain('/artifacts')
    expect(allTos).not.toContain('/agents')
    expect(allTos).not.toContain('/sandboxes')
    expect(allTos).not.toContain('/board')
    expect(allTos).not.toContain('/integrations')
    expect(allTos).not.toContain('/triggers')
    expect(sidebarNavGroups.some((g) => g.titleKey === 'nav.groupConfig')).toBe(false)
  })

  it('keeps notifications and gates label keys for badges (plan g1.2)', () => {
    const items = sidebarNavGroups[0].items
    expect(items.find((i) => i.to === '/notifications')?.labelKey).toBe('nav.notifications')
    expect(items.find((i) => i.to === '/gates')?.labelKey).toBe('nav.gates')
  })
})

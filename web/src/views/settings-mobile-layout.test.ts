import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))

function read(name: string) {
  return readFileSync(join(dir, name), 'utf8')
}

describe('settings-family narrow-screen stacking (g3)', () => {
  it('Settings header stacks on mobile with full-width touch targets', () => {
    const src = read('SettingsView.vue')
    expect(src).toMatch(/flex-col items-stretch gap-3 md:flex-row md:items-end md:justify-between/)
    expect(src).toMatch(/min-h-11 w-full md:w-auto/)
  })

  it('Integrations availability badges wrap instead of shrink-0', () => {
    const src = read('IntegrationsView.vue')
    expect(src).toMatch(/flex flex-wrap items-center gap-2/)
    expect(src).toMatch(/availabilityBadgeClass\(m\.scope\)/)
    expect(src).not.toMatch(/inline-flex shrink-0 items-center gap-1 rounded-full border/)
    expect(src).toMatch(/min-h-11 items-start gap-3/)
  })

  it('Triggers chips wrap and Notifications mark-all is min-h-11', () => {
    expect(read('TriggersView.vue')).toMatch(/flex flex-wrap items-center gap-2/)
    expect(read('NotificationsView.vue')).toMatch(/min-h-11 border border-line bg-transparent/)
    expect(read('NotificationsView.vue')).toMatch(/min-h-11 border px-2.5/)
  })
})

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

  // plan g2.1 / g2.4: integrations card + modal replace IntegrationsView page layout
  it('Integrations modal availability badges wrap; settings open button keeps touch height', () => {
    const modal = readFileSync(
      join(dir, '../components/settings/IntegrationsModal.vue'),
      'utf8',
    )
    expect(modal).toMatch(/flex flex-wrap items-center gap-2/)
    expect(modal).toMatch(/availabilityBadgeClass\(m\.scope\)/)
    expect(modal).not.toMatch(/inline-flex shrink-0 items-center gap-1 rounded-full border/)
    expect(modal).toMatch(/min-h-11 items-start gap-3/)
    const settings = read('SettingsView.vue')
    expect(settings).toMatch(/data-testid="settings-integrations-open"/)
    expect(settings).toMatch(/class="min-h-11"/)
    expect(settings).not.toMatch(/TriggersView/)
  })

  it('Notifications controls keep touch height', () => {
    expect(read('NotificationsView.vue')).toMatch(/min-h-11 border border-line bg-transparent/)
    expect(read('NotificationsView.vue')).toMatch(/min-h-11 border-b-2 border-transparent px-4/)
  })
})

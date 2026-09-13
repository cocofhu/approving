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
    expect(src).toMatch(/min-h-11 w-full md:min-h-0 md:w-auto/)
    expect(src).toMatch(/size="md"/)
  })

  it('Integrations panel availability badges wrap and keeps mobile touch targets', () => {
    const panel = readFileSync(
      join(dir, '../components/settings/IntegrationsPanel.vue'),
      'utf8',
    )
    expect(panel).toMatch(/flex flex-wrap items-center gap-2/)
    expect(panel).toMatch(/availabilityBadgeClass\(m\.scope\)/)
    expect(panel).not.toMatch(/inline-flex shrink-0 items-center gap-1 rounded-full border/)
    expect(panel).toMatch(/min-h-11 items-start gap-3 md:min-h-0/)
    expect(panel).toMatch(/min-h-11 items-center gap-1[\s\S]*md:min-h-0/)
    expect(panel).toMatch(/data-testid="integrations-panel-back"/)
    expect(panel).toMatch(/mb-2 inline-flex min-h-11/)
    expect(panel).not.toMatch(/AppModal/)
    const settings = read('SettingsView.vue')
    expect(settings).not.toMatch(/data-testid="settings-integrations-open"/)
    expect(settings).not.toMatch(/data-testid="settings-integrations-card"/)
    expect(settings).toMatch(/min-h-11 w-full md:min-h-0 md:w-auto/)
    expect(settings).toMatch(/IntegrationsPanel v-if="showIntegrations"/)
    expect(settings).not.toMatch(/openIntegrations/)
    expect(settings).not.toMatch(/TriggersView/)
  })

  it('PlatformRulesView desktop header actions use size md; min-h-11 is paired with md:min-h-0', () => {
    const src = read('PlatformRulesView.vue')
    expect(src).toMatch(/variant="ghost"\s+size="md"/)
    expect(src).toMatch(/variant="primary"\s+size="md"/)
    expect(src).not.toMatch(/size="sm"/)
    const minH11 = src.match(/min-h-11/g) || []
    const paired = src.match(/min-h-11[\s\S]{0,80}md:min-h-0/g) || []
    expect(paired.length).toBe(minH11.length)
  })

  it('Notifications controls keep touch height', () => {
    expect(read('NotificationsView.vue')).toMatch(/min-h-11 border border-line bg-transparent/)
    expect(read('NotificationsView.vue')).toMatch(/min-h-11 border-b-2 border-transparent px-4/)
  })
})

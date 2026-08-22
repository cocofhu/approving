// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const badge = readFileSync(join(dir, 'ServiceCommitBadge.vue'), 'utf8')
const shell = readFileSync(join(dir, 'AppShell.vue'), 'utf8')
const shutdown = readFileSync(join(dir, '../../lib/composables/useShutdownState.ts'), 'utf8')
const toast = readFileSync(join(dir, '../ui/ToastHost.vue'), 'utf8')

describe('service commit badge f1–f5 checklist (g3.3)', () => {
  it('f1: overview-only, main-pane lower-right, grey mono 7-char, no demo chrome', () => {
    expect(badge).toMatch(/route\.name !== 'dashboard'/)
    expect(badge).toMatch(/text-txt3/)
    expect(badge).toMatch(/font-mono/)
    expect(badge).toMatch(/text-\[11px\]/)
    expect(badge).toMatch(/right-\[14px\]/)
    expect(badge).toMatch(/bottom-\[calc\(10px\+env\(safe-area-inset-bottom/)
    expect(badge).not.toMatch(/服务程序 commit/)
    expect(badge).not.toMatch(/border-dashed|purple|#7c3aed/)
    expect(shell).toMatch(/<ServiceCommitBadge\s*\/>/)
    expect(shell.indexOf('<ServiceCommitBadge')).toBeGreaterThan(shell.indexOf('class="scroll-area'))
  })

  it('f2: non-dashboard routes unmount; login/public stay on bare layout', () => {
    const app = readFileSync(join(dir, '../../App.vue'), 'utf8')
    expect(badge).toMatch(/v-if="sha"/)
    expect(app).toMatch(/bareLayout/)
    expect(app).toMatch(/<AppShell v-if="!bareLayout">/)
  })

  it('f3: data source is GET /api/health commit, not VITE_GIT_COMMIT or dashboard stats', () => {
    const apiTypes = readFileSync(join(dir, '../../lib/api/apiTypes.ts'), 'utf8')
    const settingsClient = readFileSync(join(dir, '../../lib/api/clients/settingsClient.ts'), 'utf8')
    expect(apiTypes).toMatch(/export type HealthResponse/)
    expect(apiTypes).toMatch(/commit\?: string/)
    expect(settingsClient).toMatch(/health: \(\) => req<HealthResponse>\(`\/health`\)/)
    expect(shutdown).toMatch(/applyHealthCommit\(body\.commit\)/)
    expect(badge).not.toMatch(/VITE_GIT_COMMIT/)
    expect(shell).not.toMatch(/VITE_GIT_COMMIT/)
    expect(shell).not.toMatch(/\/stats\/dashboard/)
  })

  it('f4: empty/illegal SHA does not render placeholders', () => {
    expect(badge).toMatch(/v-if="sha"/)
    expect(badge).not.toMatch(/unknown|N\/A|—/)
  })

  it('f5: pointer-events-none and z-index below ToastHost / drain toast', () => {
    expect(badge).toMatch(/pointer-events-none/)
    expect(badge).toMatch(/\bz-10\b/)
    expect(toast).toMatch(/z-\[100\]/)
    expect(shell).toMatch(/drainToast[\s\S]*z-50/)
  })
})

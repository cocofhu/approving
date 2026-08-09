// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { authRedirectPath } from '@/lib/useAuth'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'index.ts'), 'utf8')

describe('login redirect query preservation (g3.4)', () => {
  it('logged-in /login uses string redirect via authRedirectPath, not { path }', () => {
    expect(src).toMatch(/authRedirectPath/)
    expect(src).toMatch(/return authRedirectPath\(redirect\)/)
    // Vue Router does not parse ?query from object-form path.
    expect(src).not.toMatch(/return \{ path: redirect/)
    expect(src).not.toMatch(/return \{ path: redirect\.startsWith/)
  })

  it('string replace keeps node/tab query; object path drops it', async () => {
    const routes = [
      { path: '/login', component: { template: '<div />' } },
      { path: '/runs/:id', component: { template: '<div />' } },
    ]
    const keep = createRouter({ history: createMemoryHistory(), routes })
    await keep.push('/login')
    await keep.replace(authRedirectPath('/runs/r1?node=end&tab=output'))
    expect(keep.currentRoute.value.path).toBe('/runs/r1')
    expect(keep.currentRoute.value.query).toEqual({ node: 'end', tab: 'output' })

    const drop = createRouter({ history: createMemoryHistory(), routes })
    await drop.push('/login')
    await drop.replace({ path: '/runs/r1?node=end&tab=output' })
    expect(drop.currentRoute.value.query).toEqual({})
  })
})

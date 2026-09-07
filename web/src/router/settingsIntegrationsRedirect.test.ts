// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

const here = dirname(fileURLToPath(import.meta.url))
const routerSrc = readFileSync(join(here, 'index.ts'), 'utf8')

/** plan g1.2 / g1.3: old bookmarks converge into settings */
describe('settings integrations route redirects (plan g1.2 / g1.3)', () => {
  it('router source redirects /integrations and /triggers without mounting retired views', () => {
    expect(routerSrc).toMatch(/path: '\/integrations'[\s\S]*redirect:[\s\S]*\/settings/)
    expect(routerSrc).toMatch(/query: \{ integrations: '1' \}/)
    expect(routerSrc).toMatch(/path: '\/triggers'[\s\S]*redirect: '\/settings'/)
    expect(routerSrc).not.toMatch(/IntegrationsView/)
    expect(routerSrc).not.toMatch(/TriggersView/)
  })

  it('runtime: /integrations → /settings?integrations=1; /triggers → /settings', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/integrations', redirect: { path: '/settings', query: { integrations: '1' } } },
        { path: '/triggers', redirect: '/settings' },
        { path: '/settings', name: 'settings', component: { template: '<div />' } },
      ],
    })

    await router.push('/integrations')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/settings')
    expect(router.currentRoute.value.query.integrations).toBe('1')

    await router.push('/triggers')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/settings')
    expect(router.currentRoute.value.query.integrations).toBeUndefined()
  })
})

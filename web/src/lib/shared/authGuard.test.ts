// @vitest-environment happy-dom
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getAuthState, useAuth } from '../composables/useAuth'

const me = vi.fn()

vi.mock('@/lib/api/api', () => ({
  authApi: {
    me: (...args: unknown[]) => me(...args),
  },
}))

import { installAuthGuard } from './authGuard'

function resetAuth() {
  const s = getAuthState()
  s.user = null
  s.ready = false
}

describe('installAuthGuard', () => {
  beforeEach(() => {
    me.mockReset()
    resetAuth()
  })

  it('does not block navigation while /me is in flight', async () => {
    me.mockReturnValue(new Promise(() => {}))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/login', name: 'login', component: { template: '<div />' }, meta: { public: true, bare: true } },
        { path: '/dashboard', name: 'dashboard', component: { template: '<div />' } },
      ],
    })
    installAuthGuard(router)
    await router.push('/dashboard')
    expect(router.currentRoute.value.path).toBe('/dashboard')
    expect(getAuthState().ready).toBe(false)
    expect(me).toHaveBeenCalled()
  })

  it('redirects to login with redirect query when /me fails', async () => {
    me.mockRejectedValue(new Error('unauthorized'))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/login', name: 'login', component: defineComponent({ template: '<div />' }), meta: { public: true, bare: true } },
        { path: '/runs', name: 'runs', component: defineComponent({ template: '<div />' }) },
      ],
    })
    installAuthGuard(router)
    await router.push('/runs')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/runs')
    expect(getAuthState().ready).toBe(true)
  })

  it('marks auth ready when cached user exists', async () => {
    useAuth().setUser({ username: 'u', expiresAt: 't' })
    me.mockRejectedValue(new Error('should not call'))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/dashboard', name: 'dashboard', component: { template: '<div />' } }],
    })
    installAuthGuard(router)
    await router.push('/dashboard')
    expect(me).not.toHaveBeenCalled()
    expect(getAuthState().ready).toBe(true)
    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('login stays non-blocking and uses string authRedirectPath', async () => {
    me.mockReturnValue(new Promise(() => {}))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/login', name: 'login', component: { template: '<div />' }, meta: { public: true, bare: true } },
        { path: '/', name: 'home', component: { template: '<div />' } },
      ],
    })
    installAuthGuard(router)
    await router.push('/login')
    expect(router.currentRoute.value.path).toBe('/login')
    expect(getAuthState().ready).toBe(false)
  })
})

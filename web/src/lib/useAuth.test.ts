import { describe, expect, it } from 'vitest'
import { authRedirectPath, getAuthState, markAuthReady, useAuth } from './useAuth'

describe('useAuth', () => {
  it('sanitizes redirect paths and manages user state', () => {
    expect(authRedirectPath('')).toBe('/')
    expect(authRedirectPath('https://evil')).toBe('/')
    expect(authRedirectPath('//evil')).toBe('/')
    expect(authRedirectPath('/runs?x=1')).toBe('/runs?x=1')

    const auth = useAuth()
    auth.clearUser()
    expect(auth.isLoggedIn.value).toBe(false)
    expect(auth.ready.value).toBe(true)

    auth.setUser({ username: 'u', expiresAt: 't', isAdmin: true })
    expect(auth.user.value?.username).toBe('u')
    expect(auth.isLoggedIn.value).toBe(true)
    expect(getAuthState().user?.isAdmin).toBe(true)

    markAuthReady()
    expect(auth.ready.value).toBe(true)
  })
})

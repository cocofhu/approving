// @vitest-environment happy-dom
import { beforeEach, describe, expect, it } from 'vitest'
import { applyPublicLightChrome, reapplyThemeChrome, setTheme, theme, toggleTheme } from './theme'

describe('theme', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove('light')
  })

  it('sets and toggles theme with persistence', () => {
    setTheme('light')
    expect(theme.value).toBe('light')
    expect(localStorage.getItem('approving-theme')).toBe('light')
    expect(document.documentElement.classList.contains('light')).toBe(true)

    toggleTheme()
    expect(theme.value).toBe('dark')
    expect(document.documentElement.classList.contains('light')).toBe(false)
  })

  it('applyPublicLightChrome forces html.light without persisting theme', () => {
    setTheme('dark')
    applyPublicLightChrome()
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(document.documentElement.style.colorScheme).toBe('light')
    expect(theme.value).toBe('dark')
    expect(localStorage.getItem('approving-theme')).toBe('dark')

    reapplyThemeChrome()
    expect(document.documentElement.classList.contains('light')).toBe(false)
    expect(document.documentElement.style.colorScheme).toBe('dark')
    expect(localStorage.getItem('approving-theme')).toBe('dark')
  })
})

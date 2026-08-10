import { ref } from 'vue'

export type ThemeName = 'dark' | 'light'

const STORAGE_KEY = 'approving-theme'

function initial(): ThemeName {
  const saved = localStorage.getItem(STORAGE_KEY) as ThemeName | null
  if (saved === 'dark' || saved === 'light') return saved
  return 'dark'
}

export const theme = ref<ThemeName>(initial())

function apply(t: ThemeName) {
  const root = document.documentElement
  root.classList.toggle('light', t === 'light')
  root.style.colorScheme = t
}

export function setTheme(t: ThemeName) {
  theme.value = t
  localStorage.setItem(STORAGE_KEY, t)
  apply(t)
}

export function toggleTheme() {
  setTheme(theme.value === 'dark' ? 'light' : 'dark')
}

/**
 * Public external page: force light chrome (html.light + color-scheme).
 * Does not call setTheme or write approving-theme, so internal users'
 * persisted theme is not polluted.
 */
export function applyPublicLightChrome(): void {
  const root = document.documentElement
  root.classList.add('light')
  root.style.colorScheme = 'light'
}

/** Re-apply persisted/default theme after leaving a public page. */
export function reapplyThemeChrome(): void {
  apply(theme.value)
}

apply(theme.value)

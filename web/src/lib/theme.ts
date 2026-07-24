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

apply(theme.value)

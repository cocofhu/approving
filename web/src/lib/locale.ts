import { ref, watch } from 'vue'
import { i18n } from './i18n'
import {
  loadLocaleMessages,
  otherLocale,
  prefetchLocale,
  type AppLocale,
} from './loadLocaleMessages'

export type { AppLocale }

const STORAGE_KEY = 'approving-locale'

export function detectLocale(): AppLocale {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'zh-CN' || saved === 'en') return saved
  const lang = (navigator.language || 'zh-CN').toLowerCase()
  if (lang.startsWith('zh')) return 'zh-CN'
  if (lang.startsWith('en')) return 'en'
  return 'zh-CN'
}

/** Public external page: zh* → 简体中文, otherwise English. Does not persist. */
export function detectPublicLocale(): AppLocale {
  const lang = (navigator.language || '').toLowerCase()
  if (lang.startsWith('zh')) return 'zh-CN'
  return 'en'
}

export async function applyPublicLocale(): Promise<void> {
  const next = detectPublicLocale()
  const messages = await loadLocaleMessages(next)
  i18n.global.setLocaleMessage(next, messages)
  i18n.global.locale.value = next
  locale.value = next
  applyHtmlLocale(next)
}

export const locale = ref<AppLocale>(detectLocale())

function applyHtmlLocale(loc: AppLocale) {
  document.documentElement.lang = loc
}

export async function setLocale(next: AppLocale): Promise<void> {
  if (locale.value === next && i18n.global.locale.value === next) {
    localStorage.setItem(STORAGE_KEY, next)
    applyHtmlLocale(next)
    return
  }

  const messages = await loadLocaleMessages(next)
  i18n.global.setLocaleMessage(next, messages)
  i18n.global.locale.value = next
  locale.value = next
  localStorage.setItem(STORAGE_KEY, next)
  applyHtmlLocale(next)
  prefetchLocale(otherLocale(next))
}

let initPromise: Promise<void> | null = null

export function initLocale(): Promise<void> {
  if (!initPromise) {
    initPromise = (async () => {
      const initial = detectLocale()
      const messages = await loadLocaleMessages(initial)
      i18n.global.setLocaleMessage(initial, messages)
      i18n.global.locale.value = initial
      locale.value = initial
      applyHtmlLocale(initial)

      const idle = window.requestIdleCallback ?? ((cb: () => void) => setTimeout(cb, 200))
      idle(() => prefetchLocale(otherLocale(initial)))
    })()
  }
  return initPromise
}

export function updateDocumentTitle(titleKey: string | undefined) {
  const appName = i18n.global.t('shell.appName')
  document.title = titleKey ? `${i18n.global.t(titleKey)} · ${appName}` : appName
}

// Side-effect: kick off locale init on module load
void initLocale()

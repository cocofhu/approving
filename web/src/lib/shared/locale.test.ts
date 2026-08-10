// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('./loadLocaleMessages', () => ({
  loadLocaleMessages: vi.fn(async (loc: string) => ({ shell: { appName: loc === 'en' ? 'Code Flow' : '代码流' } })),
  otherLocale: (loc: string) => (loc === 'en' ? 'zh-CN' : 'en'),
  prefetchLocale: vi.fn(),
}))

vi.mock('./i18n', () => {
  const locale = { value: 'zh-CN' }
  return {
    i18n: {
      global: {
        locale,
        setLocaleMessage: vi.fn(),
        t: (key: string) => (key === 'shell.appName' ? 'Code Flow' : key),
      },
    },
  }
})

import { detectLocale, initLocale, setLocale, updateDocumentTitle, locale } from './locale'
import { loadLocaleMessages, prefetchLocale } from './loadLocaleMessages'

describe('locale', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.lang = ''
    document.title = ''
    vi.mocked(loadLocaleMessages).mockClear()
    vi.mocked(prefetchLocale).mockClear()
  })

  it('detectLocale prefers stored then navigator language', () => {
    localStorage.setItem('approving-locale', 'en')
    expect(detectLocale()).toBe('en')
    localStorage.setItem('approving-locale', 'zh-CN')
    expect(detectLocale()).toBe('zh-CN')
    localStorage.removeItem('approving-locale')
    Object.defineProperty(navigator, 'language', { configurable: true, value: 'en-US' })
    expect(detectLocale()).toBe('en')
    Object.defineProperty(navigator, 'language', { configurable: true, value: 'zh-TW' })
    expect(detectLocale()).toBe('zh-CN')
    Object.defineProperty(navigator, 'language', { configurable: true, value: 'fr-FR' })
    expect(detectLocale()).toBe('zh-CN')
  })

  it('setLocale loads messages and updates document lang', async () => {
    // Ensure we are not already on the target locale (module init may detect en).
    await setLocale('zh-CN')
    vi.mocked(prefetchLocale).mockClear()
    await setLocale('en')
    expect(locale.value).toBe('en')
    expect(localStorage.getItem('approving-locale')).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    expect(prefetchLocale).toHaveBeenCalledWith('zh-CN')

    await setLocale('en')
    expect(document.documentElement.lang).toBe('en')
  })

  it('initLocale is idempotent and updateDocumentTitle sets title', async () => {
    const p1 = initLocale()
    const p2 = initLocale()
    expect(p1).toBe(p2)
    await p1
    updateDocumentTitle('pages.runs.title')
    expect(document.title).toContain('Code Flow')
    updateDocumentTitle(undefined)
    expect(document.title).toBe('Code Flow')
  })
})

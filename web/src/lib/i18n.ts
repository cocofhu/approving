import { createI18n } from 'vue-i18n'
import type { AppLocale } from './loadLocaleMessages'

export const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN' as AppLocale,
  fallbackLocale: 'zh-CN',
  missingWarn: false,
  fallbackWarn: false,
  messages: {},
})

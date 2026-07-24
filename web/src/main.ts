import './lib/locale'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { i18n } from './lib/i18n'
import { initLocale } from './lib/locale'
import { installIdleScrollbar } from './lib/idleScrollbar'

import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'
import './styles/global.css'
import './lib/theme'

installIdleScrollbar()

async function bootstrap() {
  const localeReady = initLocale()
  const app = createApp(App).use(createPinia()).use(i18n).use(router)
  // Mount immediately so brand / login shell can paint; locale fills in as JSON arrives.
  app.mount('#app')
  await localeReady
}

void bootstrap()

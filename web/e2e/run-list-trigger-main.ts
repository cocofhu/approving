import '../src/styles/global.css'
import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import RunListView from '../src/views/RunListView.vue'

installIdleScrollbar()

async function bootstrap() {
  const params = new URLSearchParams(window.location.search)
  const locale = params.get('locale') === 'en' ? 'en' : 'zh-CN'
  await initLocale()
  await setLocale(locale)

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runs', component: RunListView },
      { path: '/runs/:id', component: { render: () => h('div', { 'data-testid': 'run-detail' }, 'detail') } },
    ],
  })
  await router.push('/runs')

  createApp({ render: () => h(RouterView) })
    .use(createPinia())
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

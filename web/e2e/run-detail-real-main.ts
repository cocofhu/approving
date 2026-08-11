import '../src/styles/global.css'
import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import RunDetailView from '../src/views/RunDetailView.vue'

installIdleScrollbar()

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runs/:id', component: RunDetailView },
      { path: '/runs', component: { render: () => h('div') } },
      { path: '/workflows/:id/edit', component: { render: () => h('div') } },
    ],
  })
  await router.push('/runs/run-responsive-e2e')

  createApp({ render: () => h(RouterView) })
    .use(createPinia())
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

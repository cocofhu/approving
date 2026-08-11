import '../src/styles/global.css'
import { createApp, h } from 'vue'
import { useRoute } from 'vue-router'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import SandboxListView from '../src/views/SandboxListView.vue'

installIdleScrollbar()

async function bootstrap() {
  await initLocale()
  const params = new URLSearchParams(window.location.search)
  await setLocale(params.get('lang') === 'en' ? 'en' : 'zh-CN')

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/sandboxes', component: SandboxListView },
      {
        path: '/sandboxes/:id/console',
        name: 'sandbox-console',
        component: {
          setup() {
            const route = useRoute()
            return () =>
              h('div', { 'data-testid': 'sandbox-console-stub', class: 'p-6 text-sm' }, [
                h('div', `path:${route.path}`),
                h('div', `tab:${String(route.query.tab || '')}`),
              ])
          },
        },
      },
    ],
  })

  await router.push('/sandboxes')

  createApp({ render: () => h(RouterView) })
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

import '../src/styles/global.css'
import { createApp, h } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import SandboxConsoleView from '../src/views/SandboxConsoleView.vue'

installIdleScrollbar()

async function syncE2eOpts() {
  const params = new URLSearchParams(window.location.search)
  const qs = new URLSearchParams()
  if (params.get('connectDelay')) qs.set('connectDelay', params.get('connectDelay')!)
  if (params.get('vncFail') === '1') qs.set('vncFail', '1')
  qs.set('resetCount', '1')
  await fetch(`/__e2e/opts?${qs.toString()}`)
}

async function bootstrap() {
  await initLocale()
  // Pin zh-CN so Playwright assertions match regardless of host/browser language.
  await setLocale('zh-CN')
  await syncE2eOpts()

  const params = new URLSearchParams(window.location.search)
  const id = Number(params.get('id') || '42')
  const tab = params.get('tab')

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/sandboxes', component: { render: () => h('div', 'sandboxes') } },
      { path: '/sandboxes/:id', component: SandboxConsoleView },
    ],
  })

  await router.push({
    path: `/sandboxes/${id}`,
    query: tab ? { tab } : {},
  })

  createApp({ render: () => h(RouterView) })
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

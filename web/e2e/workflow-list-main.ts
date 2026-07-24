import '../src/styles/global.css'
import { createApp, h } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import { setTheme } from '../src/lib/theme'
import WorkflowListView from '../src/views/WorkflowListView.vue'

installIdleScrollbar()

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')

  const params = new URLSearchParams(window.location.search)
  if (params.get('theme') === 'light') {
    setTheme('light')
  }

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/workflows', component: WorkflowListView },
      { path: '/workflows/:id/edit', component: { render: () => h('div', { 'data-testid': 'edit-page' }, 'edit') } },
      { path: '/workflows/new/edit', component: { render: () => h('div', { 'data-testid': 'new-edit-page' }, 'new') } },
      { path: '/runs/:id', component: { render: () => h('div', { 'data-testid': 'run-page' }, 'run') } },
    ],
  })

  await router.push('/workflows')

  createApp({ render: () => h(RouterView) })
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

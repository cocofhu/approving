import '../src/styles/global.css'
import { createApp, h } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import { setTheme } from '../src/lib/theme'
import ProjectListView from '../src/views/ProjectListView.vue'

installIdleScrollbar()

async function bootstrap() {
  await initLocale()
  const params = new URLSearchParams(window.location.search)
  await setLocale(params.get('lang') === 'en' ? 'en' : 'zh-CN')
  if (params.get('theme') === 'light') {
    setTheme('light')
  } else {
    setTheme('dark')
  }

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/projects', component: ProjectListView },
      {
        path: '/projects/:id',
        component: { render: () => h('div', { 'data-testid': 'project-detail-page' }, 'detail') },
      },
    ],
  })

  await router.push('/projects')

  createApp({ render: () => h(RouterView) })
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

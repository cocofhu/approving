import '../src/styles/global.css'
import { createApp, h, defineComponent } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import ToastHost from '../src/components/ui/ToastHost.vue'
import DashboardView from '../src/views/DashboardView.vue'
import RunListView from '../src/views/RunListView.vue'
import { PROJECT_CONTEXT_STORAGE_KEY } from '../src/lib/useProjectContext'
import { STATUS_FILTER_STORAGE_KEY } from '../src/lib/useStatusFilter'

installIdleScrollbar()

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')

  const params = new URLSearchParams(window.location.search)
  // Clear prior status storage so KPI jump is the sole status source.
  localStorage.removeItem(STATUS_FILTER_STORAGE_KEY)
  if (params.get('memory') === '1') {
    localStorage.setItem(PROJECT_CONTEXT_STORAGE_KEY, params.get('projectId') || 'proj-1')
  } else {
    localStorage.removeItem(PROJECT_CONTEXT_STORAGE_KEY)
  }
  // Seed multi-status storage to assert KPI jump overwrites it.
  if (params.get('seedStatus') === '1') {
    localStorage.setItem(STATUS_FILTER_STORAGE_KEY, 'running,failed')
  }

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: DashboardView },
      { path: '/runs', component: RunListView },
      {
        path: '/runs/:id',
        component: {
          render: () => h('div', { 'data-testid': 'run-detail-page' }, 'run detail'),
        },
      },
      {
        path: '/projects',
        component: { render: () => h('div', { 'data-testid': 'projects-page' }, 'projects') },
      },
    ],
  })

  await router.push('/dashboard')

  const Root = defineComponent({
    setup() {
      return () => h('div', [h(RouterView), h(ToastHost)])
    },
  })

  createApp(Root).use(i18n).use(router).mount('#app')
}

void bootstrap()

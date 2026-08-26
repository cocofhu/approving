import '../src/styles/global.css'
import { createApp, defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia } from 'pinia'
import App from '../src/App.vue'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import { installRoutePendingGuards } from '../src/lib/shared/routePending'
import { installAuthGuard } from '../src/lib/shared/authGuard'
import TokenAnalyticsView from '../src/views/TokenAnalyticsView.vue'
import { sidebarNavGroups } from '../src/data/sidebarNav'

installIdleScrollbar()

const MOCK_STATS = {
  window: '30d',
  bucketWidth: 'day',
  timezone: 'UTC',
  empty: false,
  kpi: {
    total: 9000,
    deltaPct: 10,
    inputTokens: 5000,
    outputTokens: 3000,
    cacheReadTokens: 800,
    cacheWriteTokens: 200,
    workflowTotal: 7000,
    pmTotal: 2000,
    projectCount: 1,
    runCount: 3,
    modelCount: 1,
  },
  trend: [
    {
      bucket: '2026-07-01',
      total: 9000,
      workflowTotal: 7000,
      pmTotal: 2000,
      inputTokens: 5000,
      outputTokens: 3000,
      cacheReadTokens: 800,
      cacheWriteTokens: 200,
    },
  ],
  prevTrend: [],
  composition: {
    total: 9000,
    inputTokens: 5000,
    outputTokens: 3000,
    cacheReadTokens: 800,
    cacheWriteTokens: 200,
  },
  projects: [{ projectId: 'p1', name: 'Demo', total: 9000, inputTokens: 5000, outputTokens: 3000, cacheReadTokens: 800, cacheWriteTokens: 200 }],
  modelRanking: [{ modelKey: 'm1', name: 'Model', total: 9000 }],
  nodeTypes: [{ name: 'agent', total: 9000 }],
  workflows: [{ name: 'wf', total: 9000, kind: 'workflow' }],
  heatmap: { rows: ['Model'], cols: ['Demo'], grid: [[9000]] },
  topRuns: [
    {
      runId: 'r1',
      title: 'Run',
      projectId: 'p1',
      projectName: 'Demo',
      workflowName: 'wf',
      modelKey: 'm1',
      modelName: 'Model',
      total: 9000,
    },
  ],
  projectTrends: [],
  modelTrends: [],
  filterOptions: {
    projects: [{ key: 'p1', name: 'Demo' }],
    models: [{ key: 'm1', name: 'Model' }],
  },
}

window.fetch = async (input: RequestInfo | URL) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  if (url.includes('/auth/me')) {
    return new Response(
      JSON.stringify({ username: 'e2e', expires_at: '2099-01-01T00:00:00Z', is_admin: true }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/health') || url.includes('/live')) {
    return new Response(JSON.stringify({ status: 'ok' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  if (url.includes('/stats/platform-status')) {
    return new Response(
      JSON.stringify({ running: 0, waitingHuman: 0, failed: 0, completed: 0 }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/stats/dashboard')) {
    return new Response(
      JSON.stringify({ running: 0, waitingHuman: 0, failed: 0, completed: 0 }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/stats/token')) {
    return new Response(JSON.stringify(MOCK_STATS), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  if (url.includes('/gates')) {
    return new Response(
      JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20, hasMore: false }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/projects')) {
    return new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  return new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')

  const DashboardPage = defineComponent({
    name: 'TokenAnalyticsDashboard',
    setup() {
      return () => h('div', { 'data-testid': 'shell-main-dashboard' }, '工作台内容')
    },
  })

  const DummyPage = defineComponent({
    setup() {
      return () => h('div', { 'data-testid': 'shell-main-dummy' }, 'ok')
    },
  })

  const ProjectBoardPage = defineComponent({
    props: { id: { type: String, required: true } },
    setup(props) {
      return () => h('div', { 'data-testid': 'project-board-page' }, `board:${props.id}`)
    },
  })

  const RunDetailPage = defineComponent({
    props: { id: { type: String, required: true } },
    setup(props) {
      return () => h('div', { 'data-testid': 'run-detail-page' }, `run:${props.id}`)
    },
  })

  const LoginPage = defineComponent({
    setup() {
      return () => h('div', { 'data-testid': 'login-page' }, '登录')
    },
  })

  const known = new Set(['/dashboard', '/stats', '/login'])
  const extraRoutes = sidebarNavGroups.flatMap((g) =>
    g.items
      .filter((item) => !known.has(item.to))
      .map((item) => ({
        path: item.to,
        component: DummyPage,
      })),
  )

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/login',
        name: 'login',
        component: LoginPage,
        meta: { titleKey: 'route.login', public: true, bare: true },
      },
      { path: '/', redirect: '/dashboard' },
      {
        path: '/dashboard',
        name: 'dashboard',
        component: DashboardPage,
        meta: { titleKey: 'route.dashboard' },
      },
      {
        path: '/stats',
        name: 'stats',
        component: TokenAnalyticsView,
        meta: { titleKey: 'route.stats' },
      },
      {
        path: '/projects/:id',
        name: 'project-detail',
        component: ProjectBoardPage,
        props: true,
        meta: { titleKey: 'route.projectDetail' },
      },
      {
        path: '/runs/:id',
        name: 'run-detail',
        component: RunDetailPage,
        props: true,
        meta: { titleKey: 'route.runDetail' },
      },
      ...extraRoutes,
    ],
  })

  installRoutePendingGuards(router)
  installAuthGuard(router)

  await router.push('/dashboard')

  createApp(App).use(createPinia()).use(i18n).use(router).mount('#app')
}

void bootstrap()

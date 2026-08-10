import '../src/styles/global.css'
import { createApp, defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter, useRouter } from 'vue-router'
import { createPinia } from 'pinia'
import App from '../src/App.vue'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import { installRoutePendingGuards } from '../src/lib/routePending'
import { installAuthGuard } from '../src/lib/authGuard'
import NotificationsView from '../src/views/NotificationsView.vue'
import { sidebarNavGroups } from '../src/data/sidebarNav'

installIdleScrollbar()

const params = new URLSearchParams(window.location.search)
const scene = params.get('scene') || 'with-items'

function makeRun(partial: Record<string, unknown>) {
  return {
    workflowId: 'wf',
    workflowName: (partial.workflowName as string) || '自我迭代',
    title: (partial.title as string) || 'Run',
    trigger: 'manual',
    startedAt: (partial.startedAt as string) || '2026-08-10T12:00:00Z',
    durationSec: (partial.durationSec as number) ?? 60,
    progress: 100,
    nodeRuns: {},
    artifacts: [],
    ...partial,
  }
}

const historyItems = [
  makeRun({
    id: 'run-hist-1',
    status: 'completed',
    title: '产物这里根据Run 分页 而不是产物 · 1图',
    startedAt: '2026-08-01T10:00:00Z',
    durationSec: 120,
  }),
  makeRun({
    id: 'run-hist-2',
    status: 'failed',
    title: '旧失败',
    startedAt: '2026-08-01T11:00:00Z',
    durationSec: 30,
  }),
]

const postEnableItems = [
  makeRun({
    id: 'run-new-ok',
    status: 'completed',
    title: '运行中 4 等待 1 暂停 0 失败 0 已完成',
    workflowName: '自我迭代',
    startedAt: '2026-08-10T16:00:00Z',
    durationSec: 90,
  }),
  makeRun({
    id: 'run-new-fail',
    status: 'failed',
    title: '某次失败',
    workflowName: '审批流',
    startedAt: '2026-08-10T17:00:00Z',
    durationSec: 20,
  }),
  makeRun({
    id: 'run-clean',
    status: 'completed',
    title: '干净标题无噪声',
    workflowName: '自我迭代',
    startedAt: '2026-08-10T15:00:00Z',
    durationSec: 10,
  }),
]

function poolForScene() {
  if (scene === 'empty') return []
  if (scene === 'history-only') return historyItems
  if (scene === 'post-enable') return [...postEnableItems, ...historyItems]
  return postEnableItems
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
  if (url.includes('/gates')) {
    return new Response(
      JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20, hasMore: false }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  if (url.includes('/projects')) {
    return new Response(JSON.stringify([]), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  if (url.includes('/runs/') && !url.includes('?')) {
    const id = url.split('/runs/')[1]?.split(/[?#]/)[0] || ''
    const found = [...poolForScene(), ...historyItems].find((r) => r.id === id)
    const body = found || makeRun({ id, status: 'completed', artifacts: [] })
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  if (url.includes('/runs')) {
    const items = poolForScene()
    return new Response(
      JSON.stringify({
        items,
        total: items.length,
        page: 1,
        pageSize: 50,
        hasMore: false,
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  }
  return new Response(JSON.stringify({}), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')

  // Seed enable baseline: for post-enable / with-items, baseline in past so items unread;
  // for history-only, leave unset so first enable treats history as read.
  if (scene === 'post-enable' || scene === 'with-items') {
    localStorage.setItem(
      'approving.notifications.prefs.e2e',
      JSON.stringify({ enabledAt: '2020-01-01T00:00:00Z', readIds: [] }),
    )
  } else {
    localStorage.removeItem('approving.notifications.prefs.e2e')
    localStorage.removeItem('approving.runTerminalNotifications.readIds.e2e')
  }

  const DashboardPage = defineComponent({
    name: 'NotifDashboard',
    setup() {
      return () => h('div', { 'data-testid': 'shell-main-dashboard' }, '工作台')
    },
  })

  const RunsPage = defineComponent({
    name: 'NotifRuns',
    setup() {
      return () => h('div', { 'data-testid': 'shell-main-runs' }, '运行列表页')
    },
  })

  const RunDetailPage = defineComponent({
    name: 'NotifRunDetail',
    setup() {
      const router = useRouter()
      return () =>
        h('div', { 'data-testid': 'shell-main-run-detail' }, [
          `运行详情 ${String(router.currentRoute.value.params.id || '')}`,
        ])
    },
  })

  const GatesPage = defineComponent({
    setup() {
      return () => h('div', { 'data-testid': 'shell-main-gates' }, '待审批')
    },
  })

  const DummyPage = defineComponent({
    setup() {
      return () => h('div', { 'data-testid': 'shell-main-dummy' }, 'ok')
    },
  })

  const LoginPage = defineComponent({
    setup() {
      return () => h('div', { 'data-testid': 'login-page' }, '登录')
    },
  })

  const known = new Set([
    '/dashboard',
    '/runs',
    '/notifications',
    '/gates',
    '/login',
  ])
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
      { path: '/login', name: 'login', component: LoginPage, meta: { titleKey: 'route.login', public: true, bare: true } },
      { path: '/', redirect: '/dashboard' },
      { path: '/dashboard', name: 'dashboard', component: DashboardPage, meta: { titleKey: 'route.dashboard' } },
      { path: '/runs', name: 'runs', component: RunsPage, meta: { titleKey: 'route.runs' } },
      { path: '/runs/:id', name: 'run-detail', component: RunDetailPage, meta: { titleKey: 'route.runs' } },
      {
        path: '/notifications',
        name: 'notifications',
        component: NotificationsView,
        meta: { titleKey: 'route.notifications' },
      },
      { path: '/gates', name: 'gates', component: GatesPage, meta: { titleKey: 'route.gates' } },
      ...extraRoutes,
    ],
  })

  installRoutePendingGuards(router)
  installAuthGuard(router)

  const start = params.get('start') === 'notifications' ? '/notifications' : '/dashboard'
  await router.push(start)

  createApp(App).use(createPinia()).use(i18n).use(router).mount('#app')
}

void bootstrap()

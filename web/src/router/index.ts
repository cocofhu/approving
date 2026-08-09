import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { updateDocumentTitle } from '@/lib/locale'
import { authApi } from '@/lib/api'
import { authRedirectPath, getAuthState, markAuthReady, useAuth } from '@/lib/useAuth'
import LoginView from '@/views/LoginView.vue'

const routes: RouteRecordRaw[] = [
  // Eager: /login hosts brand LCP; avoid extra async chunk on critical path
  { path: '/login', name: 'login', component: LoginView, meta: { titleKey: 'route.login', public: true, bare: true } },
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { titleKey: 'route.dashboard' } },
  { path: '/board', name: 'board', component: () => import('@/views/BoardRedirectView.vue'), meta: { titleKey: 'route.board' } },
  { path: '/projects', name: 'projects', component: () => import('@/views/ProjectListView.vue'), meta: { titleKey: 'route.projects' } },
  { path: '/projects/:id', name: 'project-detail', component: () => import('@/views/ProjectDetailView.vue'), meta: { titleKey: 'route.projectDetail' } },
  { path: '/workflows', redirect: '/projects' },
  { path: '/workflows/:id/edit', name: 'workflow-editor', component: () => import('@/views/WorkflowEditorView.vue'), meta: { titleKey: 'route.workflowEditor', full: true } },
  { path: '/runs', name: 'runs', component: () => import('@/views/RunListView.vue'), meta: { titleKey: 'route.runs' } },
  { path: '/runs/:id', name: 'run-detail', component: () => import('@/views/RunDetailView.vue'), meta: { titleKey: 'route.runDetail', full: true } },
  { path: '/gates', name: 'gates', component: () => import('@/views/GatesInboxView.vue'), meta: { titleKey: 'route.gates' } },
  { path: '/artifacts', name: 'artifacts', component: () => import('@/views/ArtifactsView.vue'), meta: { titleKey: 'route.artifacts' } },
  { path: '/agents', name: 'agents', component: () => import('@/views/AgentStudioView.vue'), meta: { titleKey: 'route.agents' } },
  { path: '/sandboxes', name: 'sandboxes', component: () => import('@/views/SandboxListView.vue'), meta: { titleKey: 'route.sandboxes' } },
  { path: '/sandboxes/:id/console', name: 'sandbox-console', component: () => import('@/views/SandboxConsoleView.vue'), meta: { titleKey: 'route.sandboxConsole', full: true } },
  { path: '/integrations', name: 'integrations', component: () => import('@/views/IntegrationsView.vue'), meta: { titleKey: 'route.integrations' } },
  { path: '/triggers', name: 'triggers', component: () => import('@/views/TriggersView.vue'), meta: { titleKey: 'route.triggers' } },
  { path: '/settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { titleKey: 'route.settings' } },
  { path: '/settings/platform-rules', name: 'platform-rules', component: () => import('@/views/PlatformRulesView.vue'), meta: { titleKey: 'route.platformRules' } },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  if (to.meta.public) {
    if (to.name === 'login') {
      const { user, setUser } = useAuth()
      if (user.value) {
        // Return a string so Vue Router parses ?query (object `{ path }` drops it).
        const redirect = typeof to.query.redirect === 'string' ? to.query.redirect : '/'
        return authRedirectPath(redirect)
      }
      // Do not await /me before painting login — brand LCP must not wait on auth RTT.
      // Keep auth.ready false until /me settles so LoginView can show brand-only shell
      // (no form / Demo credentials flash) while a session redirect may still happen.
      void authApi
        .me()
        .then((me) => {
          setUser({ username: me.username, expiresAt: me.expires_at, isAdmin: !!me.is_admin })
          const redirect = typeof to.query.redirect === 'string' ? to.query.redirect : '/'
          return router.replace(authRedirectPath(redirect))
        })
        .catch(() => {
          markAuthReady()
        })
      return true
    }
    return true
  }

  const state = getAuthState()
  const { setUser } = useAuth()
  if (!state.user) {
    try {
      const me = await authApi.me()
      setUser({ username: me.username, expiresAt: me.expires_at, isAdmin: !!me.is_admin })
    } catch {
      markAuthReady()
      return { path: '/login', query: { redirect: to.fullPath } }
    }
  } else {
    markAuthReady()
  }
  return true
})

router.afterEach((to) => {
  updateDocumentTitle(to.meta.titleKey as string | undefined)
})

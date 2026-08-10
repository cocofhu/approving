import '../src/styles/global.css'
import { createApp, h, defineComponent } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import ToastHost from '../src/components/ui/ToastHost.vue'
import RunListView from '../src/views/RunListView.vue'

installIdleScrollbar()

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/runs', component: RunListView },
      {
        path: '/runs/:id',
        component: {
          render: () => h('div', { 'data-testid': 'run-detail-page' }, 'run detail'),
        },
      },
    ],
  })
  await router.push('/runs')

  const Root = defineComponent({
    setup() {
      return () =>
        h('div', [
          h(RouterView),
          h(ToastHost),
        ])
    },
  })

  createApp(Root).use(i18n).use(router).mount('#app')
}

void bootstrap()

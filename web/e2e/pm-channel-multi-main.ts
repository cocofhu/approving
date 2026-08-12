import '../src/styles/global.css'
import { createApp, h, ref } from 'vue'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import { setTheme } from '../src/lib/shared/theme'
import PmChannelMultiPanel from '../src/components/pm/PmChannelMultiPanel.vue'
import type { Project } from '../src/lib/shared/types'

installIdleScrollbar()

const project = ref<Project>({
  id: 'proj-e2e',
  name: 'E2E Multi Channel',
  notifyPolicy: {
    enabled: true,
    defaultEvents: ['waiting_human', 'failed'],
    channelIds: ['chn-primary'],
  },
} as Project)

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')
  setTheme('light')

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { render: () => h('div') } }],
  })
  await router.push('/')

  createApp({
    setup() {
      return () =>
        h('div', { class: 'mx-auto max-w-3xl', 'data-testid': 'pm-channel-multi-root' }, [
          h('h1', { class: 'mb-4 text-lg font-semibold' }, '渠道接入 (QQ) · 多 Channel'),
          h(PmChannelMultiPanel, {
            projectId: 'proj-e2e',
            project: project.value,
            pmLeaderAgent: 'agent-primary',
            'onProject-updated': (p: Project) => {
              project.value = p
            },
          }),
        ])
    },
  })
    .use(createPinia())
    .use(i18n)
    .use(router)
    .mount('#app')
}

void bootstrap()

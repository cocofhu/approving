import '../src/styles/global.css'
import { createApp, defineComponent, h } from 'vue'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { installIdleScrollbar } from '../src/lib/shared/idleScrollbar'
import { setTheme } from '../src/lib/shared/theme'
import CitationCard from '../src/components/pm/CitationCard.vue'
import type { ProgressCitation } from '../src/lib/shared/types'

installIdleScrollbar()

const Fixture = defineComponent({
  name: 'PmCitationFpFixture',
  setup() {
    const invalid: ProgressCitation = {
      type: 'run',
      targetId: 'trigger',
      summarySnippet: 'run:trigger',
    }
    const valid: ProgressCitation = {
      type: 'run',
      targetId: 'run-a1b2c3d4',
      summarySnippet: '需求澄清 · 进行中',
    }
    return () =>
      h(
        'div',
        { class: 'mx-auto flex max-w-xl flex-col gap-6', 'data-testid': 'pm-citation-fp-root' },
        [
          h('section', { 'data-testid': 'scene-history-invalid' }, [
            h('p', { class: 'mb-2 text-xs text-txt3' }, '历史假 citation（改后）'),
            h(CitationCard, { citation: invalid }),
          ]),
          h('section', { 'data-testid': 'scene-valid' }, [
            h('p', { class: 'mb-2 text-xs text-txt3' }, '合法引用'),
            h(CitationCard, { citation: valid }),
          ]),
        ],
      )
  },
})

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')
  setTheme('light')

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: Fixture },
      { path: '/runs/:id', component: { render: () => h('div', { 'data-testid': 'run-nav' }, 'run') } },
    ],
  })
  await router.push('/')

  createApp(Fixture).use(createPinia()).use(i18n).use(router).mount('#app')
}

void bootstrap()

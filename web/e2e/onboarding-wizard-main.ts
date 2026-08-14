import '../src/styles/global.css'
import { createApp, h, ref } from 'vue'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale, setLocale } from '../src/lib/shared/locale'
import { setTheme } from '../src/lib/shared/theme'
import OnboardingWizard from '../src/components/onboarding/OnboardingWizard.vue'

async function boot() {
  await initLocale()
  const params = new URLSearchParams(window.location.search)
  await setLocale(params.get('lang') === 'en' ? 'en' : 'zh-CN')
  setTheme('dark')

  const open = ref(true)
  const app = createApp({
    setup() {
      return () =>
        h('div', [
          // Mirrors ProjectDetailView empty CTA copy for shell i18n e2e assertions.
          h(
            'p',
            { 'data-testid': 'onboarding-empty-desc' },
            String(i18n.global.t('pages.onboarding.emptyDesc')),
          ),
          h(OnboardingWizard, {
            open: open.value,
            projectId: 'e2e-project',
            onClose: () => {
              open.value = false
            },
          }),
        ])
    },
  })
  app.use(i18n)
  app.mount('#app')
  document.getElementById('app')?.setAttribute('data-testid', 'onboarding-wizard-root')
}

void boot()

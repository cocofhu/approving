import '../src/styles/global.css'
import { createApp, h, ref } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { setTheme } from '../src/lib/theme'
import OnboardingWizard from '../src/components/onboarding/OnboardingWizard.vue'

async function boot() {
  await initLocale()
  await setLocale('zh-CN')
  setTheme('dark')

  const open = ref(true)
  const app = createApp({
    setup() {
      return () =>
        h('div', [
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

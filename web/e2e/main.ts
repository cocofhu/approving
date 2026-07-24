import '../src/styles/global.css'
import { createApp } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import AppPreviewPanel from '../src/components/run/AppPreviewPanel.vue'

async function syncE2eOpts() {
  const params = new URLSearchParams(window.location.search)
  const qs = new URLSearchParams()
  if (params.get('connectDelay')) qs.set('connectDelay', params.get('connectDelay')!)
  if (params.get('vncFail') === '1') qs.set('vncFail', '1')
  qs.set('resetCount', '1')
  await fetch(`/__e2e/opts?${qs.toString()}`)
}

async function bootstrap() {
  await initLocale()
  await setLocale('zh-CN')
  await syncE2eOpts()
  createApp(AppPreviewPanel, {
    runId: 'run-e2e',
    nodeId: 'node-e2e',
    compact: true,
  })
    .use(i18n)
    .mount('#app')
}

void bootstrap()

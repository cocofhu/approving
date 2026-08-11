import '../src/styles/global.css'
import { createApp, defineComponent, h, ref } from 'vue'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale } from '../src/lib/shared/locale'
import { setTheme } from '../src/lib/shared/theme'
import ClarifyChat from '../src/components/run/ClarifyChat.vue'
import type { ClarifyImage, ClarifyTurn, ReactAnnotation } from '../src/lib/shared/types'

initLocale()
setTheme('light')

/** Distinct 240×160 color blocks for multi-image title/src assertions + readable screenshots. */
const PNG_A =
  'iVBORw0KGgoAAAANSUhEUgAAAPAAAACgCAIAAAC9uXYyAAABRElEQVR42u3SQQ0AAAgDsQmbfxWIQQUf0qQKLpdp4Y1IgKHB0GBoMDSGBkODocHQYGgMDYYGQ4OhwdAYGgwNhgZDg6ExNBgaDA2GBkNjaDA0GBoMDYbG0GBoMDQYGgyNocHQYGgwNIYGQ4OhwdBgaAwNhgZDg6HB0BgaDA2GBkODoTE0GBoMDYYGQ2NoMDQYGgwNhsbQYGgwNBgaDI2hwdBgaDA0GBpDg6HB0GBoDA2GBkODocHQGBoMDYYGQ4OhMTQYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgMrQKGBkODocHQGBoMDYYGQ4OhMTQYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgwNIYGQ4OhwdAYGgwNhgZDg6ExNBgaDA03FrNxQ6p/RCs5AAAAAElFTkSuQmCC'
const PNG_B =
  'iVBORw0KGgoAAAANSUhEUgAAAPAAAACgCAIAAAC9uXYyAAABRElEQVR42u3SQQ0AAAjEsJODJtQhFRV8SJMqWJaahjciAYYGQ4OhwdAYGgwNhgZDg6ExNBgaDA2GBkNjaDA0GBoMDYbG0GBoMDQYGgyNocHQYGgwNBgaQ4OhwdBgaDA0hgZDg6HB0BgaDA2GBkODoTE0GBoMDYYGQ2NoMDQYGgwNhsbQYGgwNBgaDI2hwdBgaDA0GBpDg6HB0GBoMDSGBkODocHQYGgMDYYGQ4OhMTQYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgwNIYGQ4OhwdBgaAwNhgZDg6ExtAoYGgwNhgZDY2gwNBgaDA2GxtBgaDA0GBoMjaHB0GBoMDQYGkODocHQYGgwNIYGQ4OhwdBgaAwNhgZDg6HB0BgaDA2GBkNjaDA0GBoMDYbG0GBoMDTcWANS9egxNJk8AAAAAElFTkSuQmCC'

const App = defineComponent({
  name: 'ClarifyImagePreviewHarness',
  setup() {
    const turns = ref<ClarifyTurn[]>([
      {
        role: 'agent',
        text: '请补充截图说明问题。',
        at: '2026-07-28T00:00:00Z',
        images: [{ data: PNG_A, mimeType: 'image/png' }],
      },
      {
        role: 'human',
        text: '修改 你到底看了项目吗',
        at: '2026-07-28T00:01:00Z',
        images: [
          { data: PNG_A, mimeType: 'image/png' },
          { data: PNG_B, mimeType: 'image/png' },
        ],
        annotations: [{ label: '整字段', note: '#reject' } satisfies ReactAnnotation],
      },
    ])

    const attachments = ref<ClarifyImage[]>([{ data: PNG_A, mimeType: 'image/png' }])

    return () =>
      h(
        'div',
        {
          class: 'min-h-screen p-4 max-w-3xl mx-auto',
          'data-testid': 'clarify-image-preview-root',
        },
        [
          h('h1', { class: 'mb-3 text-lg font-semibold' }, 'ClarifyChat 图片预览验收'),
          h(ClarifyChat, {
            runId: 'run-image-preview-e2e',
            nodeId: 'react-1',
            iteration: 1,
            turns: turns.value,
            done: false,
            active: true,
            reviewMode: true,
            annotateEnabled: true,
            attachments: attachments.value,
            'onUpdate:attachments': (v: ClarifyImage[]) => {
              attachments.value = v
            },
          }),
        ],
      )
  },
})

createApp(App).use(i18n).mount('#app')

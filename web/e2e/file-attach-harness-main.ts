import '../src/styles/global.css'
import { createApp, defineComponent, h, ref } from 'vue'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale } from '../src/lib/shared/locale'
import { setTheme } from '../src/lib/shared/theme'
import ClarifyChat from '../src/components/run/ClarifyChat.vue'
import type { ClarifyImage, ClarifyTurn } from '../src/lib/shared/types'

initLocale()
setTheme('light')

const PDF_TINY =
  'JVBERi0xLjQKJeLjz9MKMSAwIG9iago8PAovVHlwZSAvQ2F0YWxvZwovUGFnZXMgMiAwIFIKPj4KZW5kb2JqCjIgMCBvYmoKPDwKL1R5cGUgL1BhZ2VzCi9LaWRzIFszIDAgUl0KL0NvdW50IDEKPD4KZW5kb2JqCjMgMCBvYmoKPDwKL1R5cGUgL1BhZ2UKL01lZGlhQm94IFswIDAgMjAwIDIwMF0KL1BhcmVudCAyIDAgUgo+PgplbmRvYmoKeHJlZgowIDQKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDE1IDAwMDAwIG4gCjAwMDAwMDAwNjQgMDAwMDAgbiAKMDAwMDAwMDEyMSAwMDAwMCBuIAp0cmFpbGVyCjw8Ci9TaXplIDQKL1Jvb3QgMSAwIFIKPj4Kc3RhcnR4cmVmCjE5MAolJUVPRgo='

const App = defineComponent({
  name: 'FileAttachHarness',
  setup() {
    const turns = ref<ClarifyTurn[]>([
      {
        role: 'human',
        text: '请先看这份 PDF',
        at: '2026-08-05T00:00:00Z',
        images: [{ data: PDF_TINY, mimeType: 'application/pdf', name: '需求说明-v3.pdf' }],
      },
      {
        role: 'agent',
        text: '正在澄清本次需求。请选择材料类型，并把参考文件从下方输入框发给我。',
        at: '2026-08-05T00:01:00Z',
        questions: [
          {
            id: 'q1',
            prompt: '本次需求需要我优先阅读哪类材料？',
            options: [
              { id: 'o1', label: '现有需求文档（PDF/Word）', recommended: true },
              { id: 'o2', label: '日志与报错包' },
              { id: 'o3', label: '界面截图' },
            ],
          },
        ],
      },
    ])

    const attachments = ref<ClarifyImage[]>([])
    const lastSent = ref<{ text: string; images: ClarifyImage[] } | null>(null)

    return () =>
      h(
        'div',
        {
          class: 'min-h-screen p-4 max-w-3xl mx-auto',
          'data-testid': 'file-attach-root',
        },
        [
          h('h1', { class: 'mb-3 text-lg font-semibold' }, 'ClarifyChat 任意类型附件验收'),
          h(
            'pre',
            {
              class: 'mb-3 rounded border border-line bg-elevated p-2 text-[11px] text-txt2',
              'data-testid': 'file-attach-last-sent',
            },
            lastSent.value ? JSON.stringify(lastSent.value, null, 2) : '(尚未发送)',
          ),
          h(ClarifyChat, {
            runId: 'run-file-attach-e2e',
            nodeId: 'react-1',
            iteration: 1,
            turns: turns.value,
            done: false,
            active: true,
            reviewMode: true,
            annotateEnabled: false,
            attachments: attachments.value,
            'onUpdate:attachments': (v: ClarifyImage[]) => {
              attachments.value = v
            },
            onSend: (text: string, images: ClarifyImage[]) => {
              lastSent.value = { text, images: images || [] }
              turns.value = [
                ...turns.value,
                {
                  role: 'human',
                  text: text || '(仅附件)',
                  at: new Date().toISOString(),
                  images,
                },
              ]
            },
          }),
        ],
      )
  },
})

createApp(App).use(i18n).mount('#app')

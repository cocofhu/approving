import '../src/styles/global.css'
import { createApp, defineComponent, h, ref } from 'vue'
import { i18n } from '../src/lib/shared/i18n'
import { initLocale } from '../src/lib/shared/locale'
import { setTheme } from '../src/lib/shared/theme'
import CompositeImageStrip from '../src/components/ui/CompositeImageStrip.vue'
import ChatImageThumb from '../src/components/ui/ChatImageThumb.vue'
import ChatImagePreviewModal from '../src/components/ui/ChatImagePreviewModal.vue'
import { useChatImagePreview } from '../src/lib/composables/useChatImagePreview'
import { resetBlobMissingCacheForTests } from '../src/lib/shared/blobMissingCache'

initLocale()
setTheme('dark')

const ORPHAN_A = 'e54381fb9ce8471dbe0765d99fc0239f'
const ORPHAN_B = '5b32f70529a64bdebafade19ca497a35'
const ORPHAN_C = '6f70eb9a67f2432983d16bc26a1bb420'

function compositeValue(ids: string[]) {
  return {
    text: '合成图',
    images: ids.map((id, i) => ({
      mime: 'image/png',
      ref: `blob:${id}`,
      name: `orphan-${i}.png`,
    })),
  }
}

const App = defineComponent({
  name: 'Blob404CacheHarness',
  setup() {
    const poll = ref(0)
    const stripValue = ref(compositeValue([ORPHAN_A, ORPHAN_B, ORPHAN_C]))
    const { preview, openChatImagePreview, closeChatImagePreview } = useChatImagePreview()

    function doPoll() {
      poll.value += 1
      // New object identity, same blob ids — mirrors RunDetailView run.value = r
      stripValue.value = compositeValue([ORPHAN_A, ORPHAN_B, ORPHAN_C])
    }

    // Expose for Playwright
    ;(window as unknown as { __blob404Harness: { doPoll: () => void; resetCache: () => void } }).__blob404Harness =
      {
        doPoll,
        resetCache: () => resetBlobMissingCacheForTests(),
      }

    return () =>
      h('div', { class: 'min-h-screen space-y-4 p-4', 'data-testid': 'blob-404-root' }, [
        h('h1', { class: 'text-lg font-semibold' }, '孤儿 blob 负向缓存 · E2E'),
        h('div', { class: 'flex flex-wrap gap-2' }, [
          h(
            'button',
            {
              type: 'button',
              class: 'border border-line px-3 py-1.5 text-sm',
              'data-testid': 'btn-poll',
              onClick: doPoll,
            },
            `模拟轮询 #${poll.value}`,
          ),
          h(
            'button',
            {
              type: 'button',
              class: 'border border-line px-3 py-1.5 text-sm',
              'data-testid': 'btn-open-preview-a',
              onClick: () => openChatImagePreview(`/api/blobs/${ORPHAN_A}`, 'e54381fb…'),
            },
            '打开预览 e543…',
          ),
          h(
            'span',
            { class: 'text-sm text-txt3', 'data-testid': 'poll-count' },
            `poll=${poll.value}`,
          ),
        ]),
        h('section', { class: 'space-y-2', 'data-testid': 'panel-vars' }, [
          h('div', { class: 'text-xs text-txt3' }, 'VariablesPanel 条带'),
          h(CompositeImageStrip, { value: stripValue.value, size: 'lg' }),
        ]),
        h('section', { class: 'space-y-2', 'data-testid': 'panel-out' }, [
          h('div', { class: 'text-xs text-txt3' }, 'NodeOutputPanel 条带（同 id）'),
          h(CompositeImageStrip, { value: stripValue.value, size: 'lg' }),
        ]),
        h('section', { class: 'space-y-2', 'data-testid': 'panel-chat' }, [
          h('div', { class: 'text-xs text-txt3' }, 'ChatImageThumb（可重试）'),
          h('div', { class: 'flex flex-wrap gap-2' }, [
            h(ChatImageThumb, {
              src: `/api/blobs/${ORPHAN_A}`,
              label: 'e54381fb…',
              mode: 'previewable',
              size: 'md',
              testId: 'chat-thumb-a',
              onPreview: () => openChatImagePreview(`/api/blobs/${ORPHAN_A}`, 'e54381fb…'),
            }),
            h(ChatImageThumb, {
              src: `/api/blobs/${ORPHAN_B}`,
              label: '5b32f705…',
              mode: 'previewable',
              size: 'md',
              testId: 'chat-thumb-b',
              onPreview: () => openChatImagePreview(`/api/blobs/${ORPHAN_B}`, '5b32f705…'),
            }),
          ]),
        ]),
        h(ChatImagePreviewModal, {
          open: !!preview.value,
          src: preview.value?.src || '',
          label: preview.value?.label || '',
          testIdPrefix: 'blob-preview',
          onClose: closeChatImagePreview,
        }),
      ])
  },
})

createApp(App).use(i18n).mount('#app')

import '../src/styles/global.css'
import { createApp, defineComponent, h, nextTick, onMounted, ref } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale } from '../src/lib/locale'
import { setTheme } from '../src/lib/theme'
import ClarifyChat from '../src/components/run/ClarifyChat.vue'

initLocale()
setTheme('light')

/**
 * Clarify (non-reviewMode) harness — Demo Cancel keep-queue + refresh resume.
 * Contrasts with review-session-ux (Cancel clears queue / #77).
 */
const App = defineComponent({
  name: 'ClarifySessionUxHarness',
  setup() {
    const chatRef = ref<{
      applyReviewFrame?: (f: Record<string, unknown>) => void
      applyAcpEvents?: (e: { kind: string; text: string }[]) => void
    } | null>(null)

    onMounted(async () => {
      await nextTick()
      ;(window as unknown as { __clarifyChat: typeof chatRef.value }).__clarifyChat = chatRef.value
    })

    async function simTurnBeginStream() {
      const c = chatRef.value
      if (!c?.applyReviewFrame) return
      c.applyReviewFrame({
        event: 'turn_begin',
        item: { text: '澄清意见甲（队列首条）' },
        nodeId: 'clarify',
      })
      await nextTick()
      c.applyAcpEvents?.([{ kind: 'thought', text: '思考增量…' }])
      await nextTick()
      c.applyAcpEvents?.([{ kind: 'message', text: '流式产出正文（非整轮一次性）。' }])
    }

    async function simRefreshResume() {
      const c = chatRef.value
      if (!c?.applyReviewFrame) return
      // Simulate page reload: authoritative queue_state with busy + activeItem.
      c.applyReviewFrame({
        event: 'queue_state',
        nodeId: 'clarify',
        waiting: 1,
        items: [{ text: '澄清意见乙（仍在队列）' }],
        busy: true,
        activeItem: { text: '澄清意见甲（队列首条）' },
      })
      await nextTick()
      c.applyAcpEvents?.([{ kind: 'message', text: '刷新后续上的流式正文。' }])
    }

    return () =>
      h('div', { class: 'min-h-screen p-4 max-w-3xl mx-auto', 'data-testid': 'clarify-ux-root' }, [
        h('div', { class: 'flex gap-2 mb-3 flex-wrap' }, [
          h(
            'button',
            {
              type: 'button',
              'data-testid': 'sim-turn-stream',
              class: 'px-3 py-1.5 rounded border text-sm',
              onClick: () => void simTurnBeginStream(),
            },
            '模拟 turn_begin+流式',
          ),
          h(
            'button',
            {
              type: 'button',
              'data-testid': 'sim-refresh-resume',
              class: 'px-3 py-1.5 rounded border text-sm',
              onClick: () => void simRefreshResume(),
            },
            '模拟刷新续传',
          ),
        ]),
        h(ClarifyChat, {
          ref: chatRef,
          runId: 'run-clarify-e2e',
          nodeId: 'clarify',
          iteration: 1,
          turns: [],
          done: false,
          active: true,
          reviewMode: false,
          annotateEnabled: true,
          hideFinish: true,
          sendLabel: '发送澄清回复',
        }),
      ])
  },
})

createApp(App).use(i18n).mount('#app')

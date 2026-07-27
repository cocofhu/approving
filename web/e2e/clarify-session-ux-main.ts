import '../src/styles/global.css'
import { createApp, defineComponent, h, nextTick, onMounted, ref } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale } from '../src/lib/locale'
import { setTheme } from '../src/lib/theme'
import ClarifyChat from '../src/components/run/ClarifyChat.vue'
import type { ClarifyTurn } from '../src/lib/types'

initLocale()
setTheme('light')

/**
 * Clarify (non-reviewMode) harness — Demo Cancel keep-queue + refresh resume.
 * Contrasts with review-session-ux (Cancel clears queue / #77).
 * Also covers host integration: props.turns catch-up while live streaming (no double human).
 */
const App = defineComponent({
  name: 'ClarifySessionUxHarness',
  setup() {
    const turns = ref<ClarifyTurn[]>([])
    const hostSkipRefresh = ref(0)
    const hostDidRefresh = ref(0)
    const chatRef = ref<{
      applyReviewFrame?: (f: Record<string, unknown>) => void
      applyAcpEvents?: (e: { kind: string; text: string }[]) => void
      isSessionBusy?: () => boolean
    } | null>(null)

    /** Host narrow-update gate mirror (RunDetail/GatesInbox busy skip). */
    function maybeHostRefresh(reason: string) {
      const busy = !!chatRef.value?.isSessionBusy?.()
      if (busy) {
        hostSkipRefresh.value += 1
        return
      }
      hostDidRefresh.value += 1
      void reason
    }

    onMounted(async () => {
      await nextTick()
      ;(window as unknown as { __clarifyChat: typeof chatRef.value }).__clarifyChat = chatRef.value
    })

    async function simTurnBeginStream() {
      const c = chatRef.value
      if (!c?.applyReviewFrame) return
      // Real pump order: queue_state(remaining, busy) then turn_begin(active).
      // Dual-send harness leaves 甲+乙 in local queue; trim to 乙 before begin.
      c.applyReviewFrame({
        event: 'queue_state',
        nodeId: 'clarify',
        waiting: 1,
        items: [{ text: '澄清意见乙（仍在队列）' }],
        busy: true,
      })
      await nextTick()
      c.applyReviewFrame({
        event: 'turn_begin',
        item: { text: '澄清意见甲（队列首条）' },
        nodeId: 'clarify',
      })
      await nextTick()
      // Host would see react WS here — must skip full refresh while busy.
      maybeHostRefresh('react')
      c.applyAcpEvents?.([{ kind: 'thought', text: '思考增量…' }])
      await nextTick()
      c.applyAcpEvents?.([{ kind: 'message', text: '流式产出正文（非整轮一次性）。' }])
    }

    /** Dual-message pump frame order probe (review v2): assert 甲 live, 乙 queued. */
    async function simPumpOrderDual() {
      const c = chatRef.value
      if (!c?.applyReviewFrame) return
      c.applyReviewFrame({
        event: 'queue_state',
        nodeId: 'clarify',
        waiting: 1,
        items: [{ text: '澄清意见乙（仍在队列）' }],
        busy: true,
      })
      await nextTick()
      c.applyReviewFrame({
        event: 'turn_begin',
        item: { text: '澄清意见甲（队列首条）' },
        nodeId: 'clarify',
      })
      await nextTick()
      maybeHostRefresh('react')
    }

    /** Mid-stream: inject persisted human (softRefresh/loadRun catch-up) — must not double. */
    async function simTranscriptCatchUp() {
      const c = chatRef.value
      if (!c?.applyReviewFrame) return
      c.applyReviewFrame({
        event: 'turn_begin',
        item: { text: '澄清意见甲（队列首条）' },
        nodeId: 'clarify',
      })
      await nextTick()
      c.applyAcpEvents?.([{ kind: 'message', text: '流式产出正文（非整轮一次性）。' }])
      await nextTick()
      turns.value = [
        {
          role: 'human',
          text: '澄清意见甲（队列首条）',
          at: new Date().toISOString(),
        },
      ]
      await nextTick()
      maybeHostRefresh('react-mid-stream')
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
              'data-testid': 'sim-pump-order-dual',
              class: 'px-3 py-1.5 rounded border text-sm',
              onClick: () => void simPumpOrderDual(),
            },
            '模拟真实泵帧序',
          ),
          h(
            'button',
            {
              type: 'button',
              'data-testid': 'sim-transcript-catchup',
              class: 'px-3 py-1.5 rounded border text-sm',
              onClick: () => void simTranscriptCatchUp(),
            },
            '模拟 transcript 追上',
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
          h(
            'span',
            {
              'data-testid': 'host-skip-refresh',
              class: 'text-xs text-txt3 self-center',
            },
            `hostSkip=${hostSkipRefresh.value}`,
          ),
        ]),
        h(ClarifyChat, {
          ref: chatRef,
          runId: 'run-clarify-e2e',
          nodeId: 'clarify',
          iteration: 1,
          turns: turns.value,
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

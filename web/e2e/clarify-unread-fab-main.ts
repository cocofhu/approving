import '../src/styles/global.css'
import { createApp, defineComponent, h, ref } from 'vue'
import { i18n } from '../src/lib/i18n'
import { initLocale, setLocale } from '../src/lib/locale'
import { installIdleScrollbar } from '../src/lib/idleScrollbar'
import { setTheme } from '../src/lib/theme'
import ClarifyChat from '../src/components/run/ClarifyChat.vue'
import type { ClarifyTurn } from '../src/lib/types'

function seedTurns(): ClarifyTurn[] {
  const base = '2026-07-18T00:00:0'
  return [
    { role: 'agent', text: '已收到澄清问题：请确认未读胶囊是否仅在离底时出现。', at: `${base}0Z` },
    { role: 'human', text: '确认。贴底时不要显示 FAB。', at: `${base}1Z` },
    { role: 'agent', text: '好的。接下来我会继续输出多轮 ReAct 结果；你可以上翻阅读历史。', at: `${base}2Z` },
    {
      role: 'agent',
      text: 'Thought: 需要先改 stick-to-bottom，再叠加未读计数。\nAction: 对齐近底阈值（48px）。',
      at: `${base}3Z`,
    },
    {
      role: 'agent',
      text: 'Observation: 现网 ClarifyChat 在 turns 变更时无条件 scrollDown，与附件场景冲突。',
      at: `${base}4Z`,
    },
    { role: 'human', text: '那离底后新 turn 应该累计数字，thinking 不要算。', at: `${base}5Z` },
    {
      role: 'agent',
      text: '已记录。胶囊无可见文案，仅图标 + 精确数字，并提供 aria-label。',
      at: `${base}6Z`,
    },
    {
      role: 'agent',
      text: '下一轮将演示：你上翻后，我会在底部继续追加消息。',
      at: `${base}7Z`,
    },
    { role: 'agent', text: '补充一轮以便列表足够长，方便上翻离开底部。', at: `${base}8Z` },
    { role: 'agent', text: '再补一条历史消息，确保 scroller 可滚动。', at: `${base}9Z` },
  ]
}

const Fixture = defineComponent({
  name: 'ClarifyUnreadFabFixture',
  setup() {
    const turns = ref<ClarifyTurn[]>(seedTurns())
    const draft = ref('')
    const agentSeq = ref(0)
    const status = ref('ready')

    function leaveBottom() {
      const el = document.querySelector('[data-testid="clarify-scroller"]') as HTMLElement | null
      if (!el) return
      el.scrollTop = Math.max(0, el.scrollHeight - el.clientHeight - 160)
      el.dispatchEvent(new Event('scroll'))
      status.value = 'off-bottom'
    }

    function addAgent(n = 1) {
      for (let i = 0; i < n; i++) {
        agentSeq.value += 1
        turns.value = [
          ...turns.value,
          {
            role: 'agent',
            text: `Agent 新消息 #${agentSeq.value}：本轮 ReAct 产出已追加到对话底部。`,
            at: new Date(Date.UTC(2026, 6, 18, 1, agentSeq.value, 0)).toISOString(),
          },
        ]
      }
      status.value = `added-${n}`
    }

    function addMany(n: number) {
      addAgent(n)
    }

    ;(window as unknown as { __clarifyFab: Record<string, unknown> }).__clarifyFab = {
      leaveBottom,
      addAgent,
      addMany,
      getUnreadText: () =>
        document.querySelector('[data-testid="clarify-unread-fab"]')?.textContent?.trim() ?? '',
      fabVisible: () => !!document.querySelector('[data-testid="clarify-unread-fab"]'),
      status: () => status.value,
      turnCount: () => turns.value.length,
    }

    return () =>
      h(
        'div',
        {
          'data-testid': 'clarify-unread-fab-root',
          class: 'mx-auto flex h-full max-w-[720px] flex-col bg-base p-4',
        },
        [
          h('div', { class: 'mb-2 flex flex-wrap gap-2 text-[12px]' }, [
            h(
              'button',
              {
                type: 'button',
                'data-testid': 'btn-leave-bottom',
                class: 'rounded border border-line bg-surface px-2 py-1',
                onClick: leaveBottom,
              },
              '上翻离底',
            ),
            h(
              'button',
              {
                type: 'button',
                'data-testid': 'btn-add-agent',
                class: 'rounded border border-line bg-surface px-2 py-1',
                onClick: () => addAgent(1),
              },
              '+1 Agent turn',
            ),
            h(
              'button',
              {
                type: 'button',
                'data-testid': 'btn-add-3',
                class: 'rounded border border-line bg-surface px-2 py-1',
                onClick: () => addMany(3),
              },
              '+3 Agent turns',
            ),
            h(
              'button',
              {
                type: 'button',
                'data-testid': 'btn-add-128',
                class: 'rounded border border-line bg-surface px-2 py-1',
                onClick: () => addMany(128),
              },
              '+128',
            ),
            h('span', { 'data-testid': 'fixture-status', class: 'text-txt3' }, status.value),
          ]),
          h(
            'div',
            {
              class: 'min-h-0 flex-1 overflow-hidden rounded-xl border border-line bg-surface',
              style: { height: '520px' },
            },
            [
              h(ClarifyChat, {
                runId: 'run-fab',
                nodeId: 'react-1',
                iteration: 1,
                turns: turns.value,
                done: false,
                active: true,
                draft: draft.value,
                'onUpdate:draft': (v: string) => {
                  draft.value = v
                },
                onSend: (text: string) => {
                  turns.value = [
                    ...turns.value,
                    {
                      role: 'human',
                      text,
                      at: new Date().toISOString(),
                    },
                  ]
                  draft.value = ''
                  status.value = 'sent'
                },
              }),
            ],
          ),
        ],
      )
  },
})

async function boot() {
  await initLocale()
  await setLocale('zh-CN')
  setTheme('light')
  installIdleScrollbar()
  createApp(Fixture).use(i18n).mount('#app')
}

void boot()

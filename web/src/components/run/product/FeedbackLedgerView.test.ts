// @vitest-environment happy-dom
import { createI18n } from 'vue-i18n'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import common from '@/locales/zh-CN/common.json'
import pages from '@/locales/zh-CN/pages.json'
import type { Artifact } from '@/lib/shared/types'
import ChatImageThumb from '../../ui/ChatImageThumb.vue'
import FeedbackLedgerView from './FeedbackLedgerView.vue'

const artifactContent = vi.hoisted(() => vi.fn())

vi.mock('@/lib/api/api', () => ({
  api: { artifactContent },
  blobContentUrl: (ref: string) => `/api/blobs/${String(ref).replace(/^blob:/, '')}`,
}))

function mountLedger(name: string, doc: unknown, artifacts: Artifact[] = []) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...common, ...pages } },
  })
  return mount(FeedbackLedgerView, {
    props: { name, doc, runId: 'run-1', artifacts },
    global: { plugins: [i18n], stubs: { Icon: true, ChatImageThumb: true } },
  })
}

const indexDoc = {
  runId: 'run-1',
  totalRounds: 2,
  counts: { review: 1, clarify: 1 },
  rounds: [
    {
      seq: 1, kind: 'clarify', node: 'clarify-1', iteration: 1, round: 1,
      at: '2026-08-13T15:00:00+08:00', actor: 'system', action: 'auto_answer',
      summary: '自动采纳推荐项:缓存 = Redis',
    },
    {
      seq: 2, kind: 'review', node: 'research-1', iteration: 2, round: 3,
      at: '2026-08-13T15:07:22+08:00', actor: 'alice', action: 'revise',
      annotations: 2, attachments: 1, summary: '第 3 条结论证据不足',
      artifact: 'feedback.review.research-1.i2r3.json',
    },
  ],
}

const roundDoc = {
  runId: 'run-1',
  kind: 'review',
  node: { id: 'research-1', label: '调研', type: 'research' },
  iteration: 2,
  round: 3,
  at: '2026-08-13T15:07:22+08:00',
  actor: { name: 'alice', callerKind: 'pm' },
  feedback: {
    text: '第 3 条结论证据不足,请补上原始链接。',
    annotations: [{ jsonPath: '$.findings[2].evidence', note: '补链接' }],
    attachments: [{ ref: 'blob:9f2c', name: 'screenshot.png', mimeType: 'image/png' }],
  },
  transcript: [
    { role: 'human', at: '2026-08-13T15:07:22+08:00', text: '补上原始链接' },
    { role: 'agent', at: '2026-08-13T15:08:02+08:00', text: '已补充 3 处引用' },
  ],
  targets: [
    { name: 'research.json', before: 'ab3f1c9d', after: 'cd91e4b7', changed: true },
    { name: 'plan.json', before: 'ee11', after: 'ee11', changed: false },
  ],
  priorRounds: [{ round: 1, kind: 'review', summary: '要求补充竞品对比' }],
  index: 'feedback_index.json',
}

describe('FeedbackLedgerView', () => {
  beforeEach(() => {
    artifactContent.mockReset()
  })

  it('renders the index as a timeline of every round', () => {
    const w = mountLedger('feedback_index.json', indexDoc)
    expect(w.text()).toContain('第 3 条结论证据不足')
    expect(w.text()).toContain('自动采纳推荐项')
    expect(w.findAll('li')).toHaveLength(2)
    // Bodies stay collapsed until asked for.
    expect(w.text()).not.toContain('已补充 3 处引用')
    w.unmount()
  })

  // Depth on demand is the whole point of one file per round: expanding is what
  // fetches the body, so a long ledger costs nothing until it is read.
  it('fetches a round product only when its row is expanded', async () => {
    artifactContent.mockResolvedValue({ content: JSON.stringify(roundDoc) })
    const artifacts = [
      { id: 'art-9', name: 'feedback.review.research-1.i2r3.json' } as Artifact,
    ]
    const w = mountLedger('feedback_index.json', indexDoc, artifacts)
    expect(artifactContent).not.toHaveBeenCalled()

    await w.get('[data-testid="feedback-round-2"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()

    expect(artifactContent).toHaveBeenCalledWith('art-9')
    expect(w.text()).toContain('已补充 3 处引用')
    expect(w.text()).toContain('research.json')
    // Unchanged products are not listed as touched by this round.
    expect(w.text()).not.toContain('plan.json')

    // Collapsing and re-expanding reuses the loaded body.
    await w.get('[data-testid="feedback-round-2"]').trigger('click')
    await w.get('[data-testid="feedback-round-2"]').trigger('click')
    expect(artifactContent).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('says so when a round has no standalone product', async () => {
    const w = mountLedger('feedback_index.json', indexDoc)
    await w.get('[data-testid="feedback-round-1"]').trigger('click')
    expect(w.text()).toContain(pages.pages.product.feedback.indexOnly)
    expect(artifactContent).not.toHaveBeenCalled()
    w.unmount()
  })

  it('reports a round that cannot be fetched instead of rendering blank', async () => {
    artifactContent.mockRejectedValueOnce(new Error('boom'))
    const artifacts = [
      { id: 'art-9', name: 'feedback.review.research-1.i2r3.json' } as Artifact,
    ]
    const w = mountLedger('feedback_index.json', indexDoc, artifacts)
    await w.get('[data-testid="feedback-round-2"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.text()).toContain('feedback.review.research-1.i2r3.json')
    w.unmount()
  })

  it('renders a single round product on its own', () => {
    const w = mountLedger('feedback.review.research-1.i2r3.json', roundDoc)
    expect(w.text()).toContain('research-1')
    expect(w.text()).toContain('第 3 条结论证据不足')
    expect(w.text()).toContain('已补充 3 处引用')
    expect(w.text()).toContain('要求补充竞品对比')
    // Attachments are references, never inlined bytes.
    expect(w.findComponent(ChatImageThumb).props('src')).toBe('/api/blobs/9f2c')
    w.unmount()
  })

  it('shows an empty ledger without rows', () => {
    const w = mountLedger('feedback_index.json', { runId: 'run-1', totalRounds: 0, rounds: [] })
    expect(w.text()).toContain(pages.pages.product.feedback.empty)
    expect(w.findAll('li')).toHaveLength(0)
    w.unmount()
  })
})

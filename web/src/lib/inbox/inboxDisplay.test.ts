import { describe, expect, it } from 'vitest'
import {
  inboxBadgeLabelKey,
  inboxBadgeTone,
  inboxRunLabel,
  inboxSecondaryLine,
  isStartingInboxItem,
} from './inboxDisplay'

describe('inboxRunLabel', () => {
  it('shows runTitle when non-empty', () => {
    expect(inboxRunLabel({ runId: 'run-abc', runTitle: '需求摘要' })).toBe('需求摘要')
  })

  it('falls back to Run #id when title missing or blank', () => {
    expect(inboxRunLabel({ runId: 'run-abc123' })).toBe('Run #abc123')
    expect(inboxRunLabel({ runId: 'run-abc123', runTitle: '  ' })).toBe('Run #abc123')
  })

  it('truncates long titles at 60 chars', () => {
    const long = '甲'.repeat(80)
    const got = inboxRunLabel({ runId: 'run-x', runTitle: long })
    expect(got.length).toBeLessThanOrEqual(61)
    expect(got.endsWith('…') || got.endsWith('...')).toBe(true)
  })
})

describe('inboxSecondaryLine', () => {
  it('joins workflow name with run label', () => {
    expect(
      inboxSecondaryLine({ workflowName: 'github-feature', runId: 'run-1', runTitle: '标题' }),
    ).toBe('github-feature · 标题')
    expect(inboxSecondaryLine({ workflowName: 'wf', runId: 'run-zz' })).toBe('wf · Run #zz')
  })
})

describe('inboxBadgeLabelKey', () => {
  it('maps gate to gateType (proposal_select / human gate)', () => {
    expect(inboxBadgeLabelKey({ type: 'gate' })).toBe('pages.gatesInbox.gateType')
  })

  it('maps clarify kind to clarifyType (react)', () => {
    expect(inboxBadgeLabelKey({ type: 'clarify', kind: 'clarify' })).toBe('pages.gatesInbox.clarifyType')
  })

  it('maps review kind to reviewType (research / proposal)', () => {
    expect(inboxBadgeLabelKey({ type: 'clarify', kind: 'review' })).toBe('pages.gatesInbox.reviewType')
  })

  it('maps app_preview kind to previewType (application preview)', () => {
    expect(inboxBadgeLabelKey({ type: 'clarify', kind: 'app_preview' })).toBe(
      'pages.gatesInbox.previewType',
    )
  })

  it('falls back to clarifyType when kind omitted', () => {
    expect(inboxBadgeLabelKey({ type: 'clarify' })).toBe('pages.gatesInbox.clarifyType')
  })

  it('maps a booting sandbox to startingType regardless of kind', () => {
    expect(inboxBadgeLabelKey({ type: 'clarify', kind: 'clarify', state: 'starting' })).toBe(
      'pages.gatesInbox.startingType',
    )
  })
})

describe('isStartingInboxItem', () => {
  it('is true only for clarify items whose sandbox is still booting', () => {
    expect(isStartingInboxItem({ type: 'clarify', state: 'starting' })).toBe(true)
    expect(isStartingInboxItem({ type: 'clarify' })).toBe(false)
    expect(isStartingInboxItem({ type: 'gate', state: 'starting' })).toBe(false)
    expect(isStartingInboxItem(null)).toBe(false)
  })
})

describe('inboxBadgeTone', () => {
  it('splits gate / preview / review / clarify for Demo badge colors', () => {
    expect(inboxBadgeTone({ type: 'gate' })).toBe('gate')
    expect(inboxBadgeTone({ type: 'clarify', kind: 'app_preview' })).toBe('preview')
    expect(inboxBadgeTone({ type: 'clarify', kind: 'review' })).toBe('review')
    expect(inboxBadgeTone({ type: 'clarify', kind: 'clarify' })).toBe('clarify')
  })
})

import { describe, expect, it } from 'vitest'
import {
  emptyFailDisplayText,
  isEmptyFailedAgent,
  isFailureAssistantText,
  isRetryableFailedAgent,
} from './clarifyEmptyFail'
import type { ClarifyTurn } from '@/lib/shared/types'

describe('clarifyEmptyFail', () => {
  it('detects empty failed agents (plan g1.1)', () => {
    expect(
      isEmptyFailedAgent({ role: 'agent', text: '', at: 't' }),
    ).toBe(true)
    expect(
      isEmptyFailedAgent({ role: 'agent', text: '', thought: 'x', at: 't' }),
    ).toBe(false)
    expect(
      isEmptyFailedAgent({
        role: 'agent',
        text: '',
        at: 't',
        questions: [{ id: 'q', prompt: '?', options: [] }],
      }),
    ).toBe(false)
    expect(
      isEmptyFailedAgent({ role: 'agent', text: '', at: 't', interrupted: true }),
    ).toBe(false)
    expect(
      isEmptyFailedAgent({ role: 'agent', text: '', at: 't', streaming: true }),
    ).toBe(false)
  })

  it('treats failure banners as retryable (plan g1.2 / edge)', () => {
    expect(isFailureAssistantText('(澄清回复失败:timeout)')).toBe(true)
    expect(isFailureAssistantText('(澄清会话已失效,自动重建沙箱失败,请稍后重试)')).toBe(
      true,
    )
    expect(isFailureAssistantText('(复审修改失败:acp chat idle timeout)')).toBe(true)
    expect(isFailureAssistantText('正常回复')).toBe(false)
    expect(
      isRetryableFailedAgent({
        role: 'agent',
        text: '(澄清回复失败:x)',
        at: 't',
      }),
    ).toBe(true)
    expect(
      isRetryableFailedAgent({
        role: 'agent',
        text: '(已中断)',
        at: 't',
        interrupted: true,
      }),
    ).toBe(false)
    expect(
      isRetryableFailedAgent({
        role: 'agent',
        text: '已完成正文',
        at: 't',
      }),
    ).toBe(false)
  })

  it('prefers error text for failure card copy', () => {
    const empty: ClarifyTurn = { role: 'agent', text: '', at: 't' }
    expect(emptyFailDisplayText(empty, '本轮没有输出')).toBe('本轮没有输出')
    expect(
      emptyFailDisplayText(
        { role: 'agent', text: '(澄清回复失败:boom)', at: 't' },
        '本轮没有输出',
      ),
    ).toBe('(澄清回复失败:boom)')
  })
})

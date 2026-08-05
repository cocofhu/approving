import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import zhCommon from '@/locales/zh-CN/common.json'
import zhPages from '@/locales/zh-CN/pages.json'
import zhMcp from '@/locales/zh-CN/mcp.json'
import enPages from '@/locales/en/pages.json'
import enMcp from '@/locales/en/mcp.json'

describe('user-facing copy remediation keys', () => {
  const zh = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...zhCommon, ...zhPages, ...zhMcp } },
  })
  const en = createI18n({
    legacy: false,
    locale: 'en',
    messages: { en: { ...enPages, ...enMcp } },
  })

  it('attachment fallback is neutral without 仅* / only', () => {
    expect(zh.global.t('pages.projectDetail.pm.imagesOnly')).toBe('附件')
    expect(zh.global.t('pages.agentChatTester.imagesOnly')).toBe('附件')
    expect(en.global.t('pages.projectDetail.pm.imagesOnly')).toBe('Attachments')
    expect(zh.global.t('pages.projectDetail.pm.imagesOnly')).not.toMatch(/仅/)
    expect(en.global.t('pages.projectDetail.pm.imagesOnly')).not.toMatch(/only/i)
    expect(zh.global.t('pages.projectDetail.pm.inputPh')).toContain('50 MiB')
  })

  it('pm status/error copy drops internal jargon', () => {
    expect(zh.global.t('pages.projectDetail.pm.busyStreaming')).not.toMatch(/Steam/i)
    expect(zh.global.t('pages.projectDetail.pm.failSandboxDesc')).not.toMatch(/自动收场/)
    expect(zh.global.t('pages.projectDetail.pm.failEmptyDesc')).not.toMatch(/未落库|空气泡/)
    expect(zh.global.t('pages.projectDetail.pm.failUnknownDesc')).not.toMatch(/无法归入/)
  })

  it('human gate canvas subtitle avoids unconditional ReAct promise', () => {
    expect(zh.global.t('pages.workflowEditor.canvas.humanGateSubtitle')).toBe('人工审批')
    expect(zh.global.t('pages.workflowEditor.canvas.appPreviewSubtitle')).toContain('等待人工确认')
    expect(zh.global.t('pages.workflowEditor.canvas.humanGateSubtitle')).not.toMatch(/ReAct/)
  })

  it('integrations subtitle is the live mcp key without env/template pile-up', () => {
    const sub = zh.global.t('mcp.integrations.subtitle')
    expect(sub).toContain('MCP')
    expect(sub).not.toMatch(/GITLAB_/)
    expect(sub).not.toMatch(/\$\{vars/)
    expect(sub).not.toMatch(/作用域注入/)
  })

  it('token empty-state drops Usage/分桶/bridge/回填 jargon', () => {
    expect(zh.global.t('pages.board.tokenStats.emptyTrendHint')).not.toMatch(/Usage|分桶|bridge|回填/)
    expect(zh.global.t('pages.board.tokenStats.emptyRankHint')).not.toMatch(/Usage|分桶|bridge|回填/)
    expect(zh.global.t('pages.board.tokenStats.modelRankHint')).not.toMatch(/分桶|bridge|回填/)
    expect(zh.global.t('pages.board.tokenStats.filledTag')).not.toMatch(/回填/)
  })
})

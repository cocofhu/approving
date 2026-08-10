import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import zhCommon from '@/locales/zh-CN/common.json'
import zhPages from '@/locales/zh-CN/pages.json'
import zhMcp from '@/locales/zh-CN/mcp.json'
import zhRoute from '@/locales/zh-CN/route.json'
import enCommon from '@/locales/en/common.json'
import enPages from '@/locales/en/pages.json'
import enMcp from '@/locales/en/mcp.json'
import enRoute from '@/locales/en/route.json'

describe('user-facing copy remediation keys', () => {
  const zh = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': { ...zhCommon, ...zhPages, ...zhMcp } },
  })
  const en = createI18n({
    legacy: false,
    locale: 'en',
    messages: { en: { ...enCommon, ...enPages, ...enMcp } },
  })

  it('admin list async-state copy matches Demo lock (creating/retry/four-state)', () => {
    expect(zh.global.t('common.buttons.creating')).toBe('创建中…')
    expect(en.global.t('common.buttons.creating')).toBe('Creating…')
    expect(zh.global.t('common.buttons.retry')).toBe('重试')
    expect(en.global.t('common.buttons.retry')).toBe('Retry')
    expect(zh.global.t('common.asyncState.loadFailedTitle')).toBe('加载失败')
    expect(en.global.t('common.asyncState.loadFailedTitle')).toBe('Failed to load')
    expect(zh.global.t('common.asyncState.loadFailedDesc')).toBe('无法获取列表，请稍后重试。')
    expect(en.global.t('common.asyncState.loadFailedDesc')).toBe('Could not fetch the list. Please retry.')
    expect(zh.global.t('common.asyncState.permissionDeniedTitle')).toBe('权限不足')
    expect(en.global.t('common.asyncState.permissionDeniedTitle')).toBe('Permission denied')
    expect(zh.global.t('common.asyncState.permissionDeniedDesc')).toBe(
      '你没有查看此资源的权限，可重试或联系管理员。',
    )
    expect(en.global.t('common.asyncState.permissionDeniedDesc')).toBe(
      'You do not have access to this resource. Retry or contact an admin.',
    )
    expect(zh.global.t('common.buttons.deleting')).toBe('删除中…')
    expect(en.global.t('common.buttons.deleting')).toBe('Deleting…')
    expect(zh.global.t('common.buttons.saving')).toBe('保存中…')
    expect(en.global.t('common.buttons.saving')).toBe('Saving…')
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

  it('gate share-link copy is bilingual without hardcoded jargon', () => {
    expect(zh.global.t('pages.gatesInbox.share.copyLink')).toBe('复制临时链接')
    expect(en.global.t('pages.gatesInbox.share.copyLink')).toBe('Copy temp link')
    expect(zh.global.t('pages.gatesInbox.share.safetyHint')).toContain('信任')
    expect(zh.global.t('pages.gatesInbox.share.safetyHint')).toContain('审批工作台')
    expect(zh.global.t('pages.gatesInbox.share.safetyHint')).toContain('可取点')
    expect(zh.global.t('pages.gatesInbox.share.safetyHint')).not.toMatch(/不是内部审批工作台|不可取点|外部一次确认页/)
    expect(en.global.t('pages.gatesInbox.share.safetyHint')).toMatch(/trust/i)
    expect(en.global.t('pages.gatesInbox.share.safetyHint')).toMatch(/workbench/i)
    expect(en.global.t('pages.gatesInbox.share.safetyHint')).not.toMatch(/cannot pick elements|one-time external confirm page/i)
    expect(zh.global.t('pages.gatesInbox.share.copyUnavailable')).toContain('重新生成或撤销')
    expect(en.global.t('pages.gatesInbox.share.copyUnavailable')).toMatch(/regenerate or revoke/i)
    expect(zh.global.t('pages.gatesInbox.share.errors.noStandardAction')).toContain('标准')
    expect(en.global.t('pages.gatesInbox.share.errors.noStandardAction')).toMatch(/approve or reject/i)
    expect(zh.global.t('pages.publicGate.badge')).toBe('外部一次决策')
    expect(en.global.t('pages.publicGate.badge')).toBe('One-time external decision')
    expect(zh.global.t('pages.publicGate.badgeReview')).toBe('外部复审')
    expect(en.global.t('pages.publicGate.badgeReview')).toBe('External review')
    expect(zh.global.t('pages.publicGate.heading')).toBe('审批工作台')
    expect(en.global.t('pages.publicGate.heading')).toBe('Approval workbench')
    expect(zh.global.t('pages.publicGate.visualProduct')).toBe('视觉网页产物')
    expect(zh.global.t('pages.publicGate.approve')).toBe('确认并流转')
    expect(en.global.t('pages.publicGate.approve')).toMatch(/Confirm and advance/i)
    expect(zh.global.t('pages.publicGate.reject')).toBe('驳回')
    expect(en.global.t('pages.publicGate.reject')).toBe('Reject')
    expect(zh.global.t('pages.publicGate.doneApproved')).toBe('已确认')
    expect(zhRoute.route.publicGateApproval).toBe('外部一次决策')
    expect(enRoute.route.publicGateApproval).toBe('One-time external decision')
    expect(zh.global.t('pages.publicGate.confirm')).toBe('确认并流转')
    expect(en.global.t('pages.publicGate.confirm')).toMatch(/Confirm and advance/i)
    expect(zh.global.t('pages.publicGate.doneConfirmed')).toBe('已确认')
    expect(en.global.t('pages.publicGate.doneConfirmed')).toBe('Confirmed')
    expect(zh.global.t('pages.gatesInbox.share.errors.reviewBusy')).toContain('复审进行中')
    expect(zh.global.t('pages.publicGate.busy')).toContain('复审进行中')
    expect(zh.global.t('pages.publicGate.validationFailed')).toContain('产物校验')
    expect(zh.global.t('pages.projectDetail.audit.callerExternal')).toBe('外部')
    expect(en.global.t('pages.projectDetail.audit.callerExternal')).toBe('External')
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
    expect(zh.global.t('pages.board.tokenStats.filledTag')).not.toMatch(/回填/)
  })

  it('model rank card copy has no unknown≠other hint (g1.3)', () => {
    expect(zh.global.te('pages.board.tokenStats.modelRankHint')).toBe(false)
    expect(en.global.te('pages.board.tokenStats.modelRankHint')).toBe(false)

    const zhRank = [
      zh.global.t('pages.board.tokenStats.modelRankTitle'),
      zh.global.t('pages.board.tokenStats.modelRankSub'),
      zh.global.t('pages.board.tokenStats.modelOther'),
      zh.global.t('pages.board.tokenStats.emptyModelRankHint'),
    ].join('\n')
    const enRank = [
      en.global.t('pages.board.tokenStats.modelRankTitle'),
      en.global.t('pages.board.tokenStats.modelRankSub'),
      en.global.t('pages.board.tokenStats.modelOther'),
      en.global.t('pages.board.tokenStats.emptyModelRankHint'),
    ].join('\n')

    expect(zhRank).toContain('模型消耗排行')
    expect(zhRank).toContain('Top10 · 其余 → other')
    expect(zhRank).toContain('other（其余模型）')
    expect(zhRank).not.toMatch(/未知\s*[≠不等].*other|与 other 不同|不是 other/)
    expect(enRank).toContain('Model usage ranking')
    expect(enRank).toContain('Top10 · rest → other')
    expect(enRank).toContain('other (remaining models)')
    expect(enRank).not.toMatch(/Unknown is not the same as other/i)
    expect(enRank).not.toMatch(/Unknown.*≠.*other/i)
  })

  it('user-facing product naming uses 项目管理 / Project Management, not PM', () => {
    const zhKeys = [
      'common.runTrigger.pmMcp',
      'pages.projectDetail.tokenTipPm',
      'pages.board.tokenStats.pm',
      'pages.projectDetail.pm.enabledMcps',
      'pages.agentStudio.dialogs.renameCascadeHint',
      'pages.agentStudio.data.context.hint',
      'mcp.pmProgress.desc',
      'mcp.pmProgress.convention',
      'mcp.pmWorkflowRead.desc',
      'mcp.pmWorkflowRead.convention',
      'mcp.pmWorkflowWrite.desc',
      'mcp.pmWorkflowWrite.convention',
      'mcp.pmAgentFs.desc',
      'mcp.pmAgentFs.convention',
    ] as const
    for (const key of zhKeys) {
      const text = zh.global.t(key)
      expect(text, key).toMatch(/项目管理/)
      expect(text, key).not.toMatch(/(?<![A-Za-z0-9_-])PM(?![A-Za-z0-9_-])/)
    }

    const enKeys = [
      'common.runTrigger.pmMcp',
      'pages.projectDetail.tokenTipPm',
      'pages.board.tokenStats.pm',
      'pages.projectDetail.pm.enabledMcps',
      'pages.projectDetail.pm.gateAutoVar',
      'pages.agentStudio.dialogs.renameCascadeHint',
      'pages.agentStudio.data.context.hint',
      'mcp.pmProgress.desc',
      'mcp.pmProgress.convention',
      'mcp.pmWorkflowRead.desc',
      'mcp.pmWorkflowRead.convention',
      'mcp.pmWorkflowWrite.desc',
      'mcp.pmWorkflowWrite.convention',
      'mcp.pmAgentFs.desc',
      'mcp.pmAgentFs.convention',
    ] as const
    for (const key of enKeys) {
      const text = en.global.t(key)
      expect(text, key).toMatch(/Project Management/)
      expect(text, key).not.toMatch(/(?<![A-Za-z0-9_-])PM(?![A-Za-z0-9_-])/)
      expect(text, key).not.toMatch(/PM-only|project PM/i)
    }

    expect(zh.global.t('common.runTrigger.pmMcp')).toBe('项目管理 MCP')
    expect(en.global.t('common.runTrigger.pmMcp')).toBe('Project Management MCP')
    expect(zh.global.t('pages.projectDetail.tokenTipWorkflow')).toBe('工作流')
    expect(en.global.t('pages.projectDetail.tokenTipWorkflow')).toBe('Workflow')

    // Token source visible copy: 工作流 / Workflow + 项目管理 / Project Management (exact Title Case)
    expect(zh.global.t('pages.board.tokenStats.workflow')).toBe('工作流')
    expect(zh.global.t('pages.projectDetail.tokenTipWorkflow')).toBe('工作流')
    expect(zh.global.t('pages.board.tokenStats.pm')).toBe('项目管理')
    expect(en.global.t('pages.board.tokenStats.workflow')).toBe('Workflow')
    expect(en.global.t('pages.projectDetail.tokenTipWorkflow')).toBe('Workflow')
    expect(en.global.t('pages.board.tokenStats.pm')).toBe('Project Management')
    // Do not verify Title Case with case-insensitive /workflow/ substring
    expect(en.global.t('pages.board.tokenStats.workflow')).not.toBe('workflow')
    expect(en.global.t('pages.projectDetail.tokenTipWorkflow')).not.toBe('workflow')

    const tokenSourceNoPmKeys = [
      'pages.board.tokenStats.workflow',
      'pages.projectDetail.tokenTipWorkflow',
      'pages.board.tokenStats.pm',
    ] as const
    for (const key of tokenSourceNoPmKeys) {
      expect(zh.global.t(key), key).not.toMatch(/(?<![A-Za-z0-9_-])PM(?![A-Za-z0-9_-])/)
      expect(en.global.t(key), key).not.toMatch(/(?<![A-Za-z0-9_-])PM(?![A-Za-z0-9_-])/)
    }

    // MCP server ids stay as protocol names
    expect(zh.global.t('mcp.pmProgress.name')).toBe('pm-progress')
    expect(en.global.t('mcp.pmProgress.name')).toBe('pm-progress')
  })
})

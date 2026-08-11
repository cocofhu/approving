/**
 * Browser acceptance: QQ multi-channel / primary-secondary PM UI (Demo-aligned).
 */
import { expect, test, type Page } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const shotDir = path.join(__dirname, '..', 'test-results', 'pm-channel-multi')

const primary = {
  id: 'chn-primary',
  name: 'ApprovingPM',
  enabled: true,
  isPrimary: true,
  agentName: 'agent-primary',
  appId: '1905253489',
  appSecretSet: true,
  turnTimeoutSeconds: 3600,
  cronDeliver: true,
  cronDeliverTarget: 'c2c:17578536B91B8F8DE1287C9616233421',
  enabledMcps: ['pm-progress', 'pm-workflow-read', 'pm-workflow-write', 'pm-agent-fs'],
  config: { sandbox: true, allowMemoryWrite: false, allowSchedulerWrite: false },
}

const secondary = {
  id: 'chn-secondary',
  name: '副机器人',
  enabled: true,
  isPrimary: false,
  agentName: 'agent-secondary',
  appId: 'app-secondary-001',
  appSecretSet: true,
  turnTimeoutSeconds: 0,
  cronDeliver: false,
  cronDeliverTarget: '',
  enabledMcps: ['pm-progress'],
  config: { sandbox: false, allowMemoryWrite: true, allowSchedulerWrite: false },
}

let channels = [primary, secondary]
let notifyChannelIds = ['chn-primary', 'chn-secondary']
let freeAgents = ['agent-free']
let lastNotifyBody: Record<string, unknown> | null = null
let lastDeleteBody: Record<string, unknown> | null = null

async function mockApis(page: Page) {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    const method = route.request().method()
    const p = url.pathname

    if (p.match(/\/projects\/[^/]+\/channels$/) && method === 'GET') {
      await route.fulfill({
        status: 200,
        json: {
          items: channels,
          secretsKeyConfigured: true,
          freeAgents,
        },
      })
      return
    }
    if (p.match(/\/projects\/[^/]+\/channels$/) && method === 'POST') {
      const body = route.request().postDataJSON() as Record<string, unknown>
      const created = {
        id: `chn-new-${Date.now()}`,
        name: String(body.name || ''),
        enabled: !!body.enabled,
        isPrimary: channels.every((c) => !c.isPrimary),
        agentName: String(body.agentName || ''),
        appId: String(body.appId || ''),
        appSecretSet: true,
        turnTimeoutSeconds: Number(body.turnTimeoutSeconds) || 0,
        cronDeliver: !!body.cronDeliver,
        cronDeliverTarget: String(body.cronDeliverTarget || ''),
        enabledMcps: Array.isArray(body.enabledMcps) ? body.enabledMcps : [],
        config: (body.config as Record<string, unknown>) || {},
      }
      channels = [...channels, created]
      freeAgents = freeAgents.filter((a) => a !== created.agentName)
      await route.fulfill({ status: 200, json: created })
      return
    }
    if (p.match(/\/projects\/[^/]+\/channels\/[^/]+$/) && method === 'PUT') {
      const id = p.split('/').pop()!
      const body = route.request().postDataJSON() as Record<string, unknown>
      channels = channels.map((c) =>
        c.id === id
          ? {
              ...c,
              name: String(body.name ?? c.name),
              enabled: body.enabled !== undefined ? !!body.enabled : c.enabled,
              agentName: String(body.agentName ?? c.agentName),
              appId: String(body.appId ?? c.appId),
              turnTimeoutSeconds: Number(body.turnTimeoutSeconds ?? c.turnTimeoutSeconds) || 0,
              cronDeliver: !!body.cronDeliver,
              cronDeliverTarget: String(body.cronDeliverTarget || ''),
              enabledMcps: Array.isArray(body.enabledMcps) ? (body.enabledMcps as string[]) : c.enabledMcps,
              config: (body.config as Record<string, unknown>) || c.config,
            }
          : c,
      )
      await route.fulfill({ status: 200, json: channels.find((c) => c.id === id) })
      return
    }
    if (p.match(/\/projects\/[^/]+\/channels\/[^/]+$/) && method === 'DELETE') {
      const id = p.split('/').pop()!
      lastDeleteBody = (route.request().postDataJSON() as Record<string, unknown>) || {}
      const target = channels.find((c) => c.id === id)
      if (target?.isPrimary) {
        const newId = String(lastDeleteBody.newPrimaryId || '')
        const confirmNone = !!lastDeleteBody.confirmNoPrimary
        if (!newId && !confirmNone) {
          await route.fulfill({ status: 400, json: { error: '需要指定新主或确认无主' } })
          return
        }
        if (newId) {
          channels = channels
            .filter((c) => c.id !== id)
            .map((c) => ({ ...c, isPrimary: c.id === newId }))
        } else {
          channels = channels.filter((c) => c.id !== id)
        }
      } else {
        channels = channels.filter((c) => c.id !== id)
      }
      await route.fulfill({ status: 200, json: { status: 'ok' } })
      return
    }
    if (p.match(/\/projects\/[^/]+$/) && method === 'GET') {
      await route.fulfill({
        status: 200,
        json: {
          id: 'proj-e2e',
          name: 'E2E Multi Channel',
          notifyPolicy: {
            enabled: true,
            defaultEvents: ['waiting_human', 'failed'],
            channelIds: notifyChannelIds,
          },
        },
      })
      return
    }
    if (p.match(/\/projects\/[^/]+$/) && (method === 'PUT' || method === 'PATCH')) {
      lastNotifyBody = route.request().postDataJSON() as Record<string, unknown>
      const pol = (lastNotifyBody.notifyPolicy || {}) as { channelIds?: string[] }
      notifyChannelIds = Array.isArray(pol.channelIds) ? pol.channelIds : []
      await route.fulfill({
        status: 200,
        json: {
          id: 'proj-e2e',
          name: 'E2E Multi Channel',
          notifyPolicy: {
            enabled: true,
            defaultEvents: ['waiting_human', 'failed'],
            channelIds: notifyChannelIds,
          },
        },
      })
      return
    }
    if (p.match(/\/projects\/[^/]+\/pm\/threads/)) {
      await route.fulfill({ status: 200, json: { items: [] } })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })
}

test.describe('QQ multi-channel primary/secondary UI', () => {
  test.beforeEach(async () => {
    channels = [structuredClone(primary), structuredClone(secondary)]
    notifyChannelIds = ['chn-primary', 'chn-secondary']
    freeAgents = ['agent-free']
    lastNotifyBody = null
    lastDeleteBody = null
  })

  test('list / edit / notify tabs + primary delete + empty notify', async ({ page }) => {
    await page.setViewportSize({ width: 1200, height: 900 })
    await mockApis(page)
    await page.goto('/pm-channel-multi.html')
    await expect(page.getByTestId('pm-channel-multi-root')).toBeVisible({ timeout: 20_000 })
    await expect(page.getByTestId('pm-channel-multi')).toBeVisible()

    // g5.1: three tabs, no "每项目一个渠道"
    await expect(page.getByTestId('channel-tab-list')).toBeVisible()
    await expect(page.getByTestId('channel-tab-edit')).toBeVisible()
    await expect(page.getByTestId('channel-tab-notify')).toBeVisible()
    await expect(page.getByText('每个项目一个渠道')).toHaveCount(0)
    await expect(page.getByText('同一项目可配置多个 QQ Channel')).toBeVisible()

    // list shows primary/secondary
    await expect(page.getByTestId('channel-panel-list')).toBeVisible()
    const rows = page.getByTestId('channel-row')
    await expect(rows).toHaveCount(2)
    await expect(rows.nth(0)).toContainText('ApprovingPM')
    await expect(rows.nth(0)).toContainText('主')
    await expect(rows.nth(0)).toContainText('agent-primary')
    await expect(rows.nth(1)).toContainText('副机器人')
    await expect(rows.nth(1)).toContainText('副')
    await expect(page.getByTestId('channel-add')).toContainText('新增副 Channel')
    await page.screenshot({ path: path.join(shotDir, '01-channel-list.png'), fullPage: true })

    // g5.2: edit secondary — channel MCP note
    await rows.nth(1).getByRole('button', { name: '编辑' }).click()
    await expect(page.getByTestId('channel-panel-edit')).toBeVisible()
    await expect(page.getByText('副 Channel · 能力与主对齐')).toBeVisible()
    await expect(page.getByText('绑定 Agent（PM）')).toBeVisible()
    await expect(page.getByText('仅本 Channel 回合使用 · 不影响 Web/门禁')).toBeVisible()
    await expect(page.getByText('会话能力')).toBeVisible()
    await page.screenshot({ path: path.join(shotDir, '02-channel-edit-secondary.png'), fullPage: true })

    // g5.4: add secondary label when primary exists
    await page.getByTestId('channel-tab-list').click()
    await expect(page.getByTestId('channel-add')).toContainText('新增副 Channel')

    // g5.3: notify tab multi-select + empty hint
    await page.getByTestId('channel-tab-notify').click()
    await expect(page.getByTestId('channel-panel-notify')).toBeVisible()
    await expect(page.getByText('项目通知 · Channel 目标')).toBeVisible()
    await expect(page.getByText('已选 2')).toBeVisible()
    await page.screenshot({ path: path.join(shotDir, '03-notify-targets.png'), fullPage: true })

    // uncheck all → empty hint, save empty list
    const checks = page.locator('[data-testid="channel-panel-notify"] input[type="checkbox"]')
    const count = await checks.count()
    for (let i = 0; i < count; i++) {
      if (await checks.nth(i).isChecked()) await checks.nth(i).uncheck()
    }
    await expect(page.getByTestId('notify-empty-hint')).toBeVisible()
    await page.getByTestId('channel-notify-save').click()
    await expect.poll(() => lastNotifyBody).not.toBeNull()
    const pol = (lastNotifyBody!.notifyPolicy || {}) as { channelIds?: string[] }
    expect(pol.channelIds).toEqual([])
    await page.screenshot({ path: path.join(shotDir, '04-notify-empty.png'), fullPage: true })

    // g5.4: delete primary opens confirm modal (no silent delete)
    await page.getByTestId('channel-tab-list').click()
    await rows.nth(0).getByRole('button', { name: '删除渠道' }).click()
    await expect(page.getByTestId('channel-delete-primary-modal')).toBeVisible()
    await expect(page.getByText('指定另一 Channel 为新主')).toBeVisible()
    await expect(page.getByText('确认无主 Channel，仅保留 PmLeader')).toBeVisible()
    await page.screenshot({ path: path.join(shotDir, '05-delete-primary-modal.png'), fullPage: true })
    await page
      .getByTestId('channel-delete-primary-modal')
      .getByRole('button', { name: '确认删除' })
      .click()
    await expect.poll(() => lastDeleteBody).not.toBeNull()
    expect(lastDeleteBody!.newPrimaryId).toBe('chn-secondary')

    // after promote, only one row and add button becomes 新增副 (still has primary)
    await expect(page.getByTestId('channel-row')).toHaveCount(1)
    await expect(page.getByTestId('channel-row')).toContainText('主')
    await expect(page.getByTestId('channel-add')).toContainText('新增副 Channel')
    await page.screenshot({ path: path.join(shotDir, '06-after-promote.png'), fullPage: true })
  })

  test('no-primary add button shows 新增主 Channel', async ({ page }) => {
    channels = [
      {
        ...structuredClone(secondary),
        isPrimary: false,
      },
    ]
    freeAgents = ['agent-primary', 'agent-free']
    await page.setViewportSize({ width: 1200, height: 800 })
    await mockApis(page)
    await page.goto('/pm-channel-multi.html')
    await expect(page.getByTestId('channel-add')).toContainText('新增主 Channel')
    await expect(page.getByText('设为主')).toHaveCount(0)
    await page.screenshot({ path: path.join(shotDir, '07-add-primary-cta.png'), fullPage: true })
  })
})

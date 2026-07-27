/**
 * P0 Run→QQ NotifyPolicy UI acceptance (mock API harness via project-detail).
 * Temporary verification for test gate; covers tab + inline override + no-channel.
 */
import { test, expect } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'

const shotDir = path.join('/tmp', 'notify-ui-shots')
fs.mkdirSync(shotDir, { recursive: true })

const MOCK_PROJECT = {
  id: 'proj-1',
  name: 'Demo Project',
  description: 'Notify UI e2e',
  sandboxEnv: [],
  variables: [],
  notifyPolicy: { enabled: true, defaultEvents: ['waiting_human', 'failed'] },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const MOCK_WORKFLOWS = [
  {
    id: 'wf-self',
    name: '自我迭代',
    description: 'heavy',
    status: 'published',
    version: 2,
    updatedAt: '2026-01-01T00:00:00Z',
    lastRunAt: '2026-01-02T00:00:00Z',
    needsRepo: false,
    notifyPolicy: { mode: 'custom', events: ['waiting_human', 'failed'] },
    nodes: [],
    edges: [],
  },
  {
    id: 'wf-light',
    name: '轻量',
    description: 'light',
    status: 'draft',
    version: 1,
    updatedAt: '2026-01-03T00:00:00Z',
    needsRepo: false,
    notifyPolicy: { mode: 'off', events: [] },
    nodes: [],
    edges: [],
  },
  {
    id: 'wf-inherit',
    name: '继承默认',
    description: '',
    status: 'published',
    version: 1,
    updatedAt: '2026-01-04T00:00:00Z',
    needsRepo: false,
    notifyPolicy: { mode: 'inherit' },
    nodes: [],
    edges: [],
  },
]

async function setupNotifyHarness(
  page: import('@playwright/test').Page,
  opts: { hasChannel?: boolean } = {},
) {
  const hasChannel = opts.hasChannel ?? false
  let savedProjectPolicy: unknown = null
  let lastWorkflowSave: unknown = null
  const workflows = structuredClone(MOCK_WORKFLOWS)

  await page.setViewportSize({ width: 1280, height: 900 })

  await page.route('**/api/projects/proj-1/channel**', async (route) => {
    if (!hasChannel) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ channel: null }),
      })
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        channel: {
          id: 'c1',
          type: 'qq',
          enabled: true,
          cronDeliver: false,
          cronDeliverTarget: 'group:123',
        },
      }),
    })
  })

  await page.route('**/api/projects/proj-1', async (route) => {
    const method = route.request().method()
    if (method === 'GET') {
      const body = {
        ...MOCK_PROJECT,
        notifyPolicy: savedProjectPolicy || MOCK_PROJECT.notifyPolicy,
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      })
      return
    }
    if (method === 'PUT' || method === 'PATCH') {
      const payload = route.request().postDataJSON() as { notifyPolicy?: unknown }
      if (payload?.notifyPolicy) savedProjectPolicy = payload.notifyPolicy
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          ...MOCK_PROJECT,
          notifyPolicy: savedProjectPolicy || MOCK_PROJECT.notifyPolicy,
        }),
      })
      return
    }
    await route.continue()
  })

  await page.route(/\/api\/workflows(\/|$|\?)/, async (route) => {
    const method = route.request().method()
    const url = route.request().url()
    if (method === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(workflows),
      })
      return
    }
    // Notify-only PATCH (preferred) — no nodes/edges in payload.
    if (method === 'PATCH' && url.includes('/notify-policy')) {
      const payload = route.request().postDataJSON() as {
        notifyPolicy?: { mode: string; events?: string[] }
      }
      const idMatch = url.match(/\/workflows\/([^/]+)\/notify-policy/)
      const id = idMatch?.[1]
      lastWorkflowSave = { id, notifyPolicy: payload?.notifyPolicy, via: 'patch-notify-policy' }
      const idx = workflows.findIndex((w) => w.id === id)
      if (idx >= 0 && payload.notifyPolicy) {
        workflows[idx] = {
          ...workflows[idx],
          notifyPolicy: {
            mode: payload.notifyPolicy.mode,
            events: payload.notifyPolicy.events || [],
          },
        }
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(idx >= 0 ? workflows[idx] : { id, status: 'published', version: 1, ...payload }),
      })
      return
    }
    if (method === 'PUT' || method === 'POST') {
      const payload = route.request().postDataJSON() as {
        id?: string
        notifyPolicy?: { mode: string; events?: string[] }
        nodes?: unknown
      }
      lastWorkflowSave = { ...payload, via: 'save-workflow' }
      const idx = workflows.findIndex((w) => w.id === payload.id)
      if (idx >= 0 && payload.notifyPolicy) {
        workflows[idx] = {
          ...workflows[idx],
          notifyPolicy: {
            mode: payload.notifyPolicy.mode,
            events: payload.notifyPolicy.events || [],
          },
        }
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(idx >= 0 ? workflows[idx] : { ...payload, status: 'draft', version: 1 }),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/runs**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0, page: 1, pageSize: 100, hasMore: false }),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/projects/proj-1/pm-leader', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enabled: false }),
    })
  })

  await page.route('**/api/projects/*/token-stats**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        window: '30d',
        bucketWidth: 'day',
        timezone: 'UTC',
        empty: true,
        trend: [],
        composition: {
          inputTokens: 0,
          outputTokens: 0,
          cacheReadTokens: 0,
          cacheWriteTokens: 0,
          total: 0,
        },
        workflows: [],
      }),
    })
  })

  await page.goto('/project-detail.html?tab=notify')
  await expect(page.getByRole('heading', { name: 'Demo Project' })).toBeVisible({ timeout: 10_000 })

  return {
    getSavedProjectPolicy: () => savedProjectPolicy,
    getLastWorkflowSave: () => lastWorkflowSave,
  }
}

test.describe('Run NotifyPolicy UI (P0)', () => {
  test('通知 Tab：总开关/默认事件/completed 灰态/无渠道提示/可保存', async ({ page }) => {
    const harness = await setupNotifyHarness(page, { hasChannel: false })

    await expect(page.getByRole('button', { name: '通知' })).toBeVisible()
    await expect(page.getByTestId('project-notify-panel')).toBeVisible()
    await expect(page.getByTestId('notify-no-channel-hint')).toBeVisible()
    await expect(page.getByTestId('notify-go-channel')).toBeVisible()
    await expect(page.getByTestId('notify-master-toggle')).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByTestId('notify-ev-waiting-human')).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByTestId('notify-ev-failed')).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByTestId('notify-ev-completed-disabled')).toBeVisible()
    await expect(
      page.getByTestId('notify-ev-completed-disabled').locator('button[disabled]'),
    ).toHaveCount(1)

    await page.screenshot({
      path: path.join(shotDir, '01-notify-tab-no-channel.png'),
      fullPage: true,
    })

    await page.getByTestId('notify-master-toggle').click()
    await expect(page.getByTestId('notify-master-toggle')).toHaveAttribute('aria-checked', 'false')
    await page.getByTestId('notify-save').click()
    await expect.poll(() => harness.getSavedProjectPolicy()).toEqual({
      enabled: false,
      defaultEvents: ['waiting_human', 'failed'],
    })

    await page.screenshot({
      path: path.join(shotDir, '02-notify-tab-master-off.png'),
      fullPage: true,
    })
  })

  test('流水线行内：off/继承/自定义 + 事件勾选即持久化', async ({ page }) => {
    const harness = await setupNotifyHarness(page, { hasChannel: true })

    await page.getByRole('button', { name: '流水线' }).click()
    await expect(page.getByTestId('wf-notify-cell').first()).toBeVisible({ timeout: 5_000 })

    await page.screenshot({
      path: path.join(shotDir, '03-workflow-inline-notify.png'),
      fullPage: true,
    })

    // 轻量已是 off；将「继承默认」切到自定义并确认落库
    const inheritRow = page.locator('tbody tr', { hasText: '继承默认' })
    await inheritRow.getByRole('button', { name: '自定义' }).click()
    await expect.poll(() => {
      const s = harness.getLastWorkflowSave() as {
        id?: string
        notifyPolicy?: { mode: string }
        via?: string
        nodes?: unknown
      } | null
      return s?.id === 'wf-inherit' ? `${s.via}:${s.notifyPolicy?.mode}:nodes=${Array.isArray(s.nodes)}` : null
    }).toBe('patch-notify-policy:custom:nodes=false')

    // 自我迭代已是 custom：取消 waiting_human，仅留 failed
    const customRow = page.locator('tbody tr', { hasText: '自我迭代' })
    const waitingCb = customRow.locator('label', { hasText: 'waiting_human' }).locator('input[type="checkbox"]')
    await expect(waitingCb).toBeChecked()
    await waitingCb.click()
    await expect.poll(() => {
      const s = harness.getLastWorkflowSave() as {
        id?: string
        notifyPolicy?: { events?: string[] }
        via?: string
      } | null
      return s?.id === 'wf-self' ? `${s.via}:${(s.notifyPolicy?.events || []).join(',')}` : ''
    }).toBe('patch-notify-policy:failed')

    await page.screenshot({
      path: path.join(shotDir, '04-workflow-custom-events.png'),
      fullPage: true,
    })
  })
})

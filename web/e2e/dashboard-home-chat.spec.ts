/**
 * E2E: Home chat starts an Approve-first pipeline and opens run detail.
 * Harness: dashboard-home-chat.html (DashboardView + run-detail stub)
 */
import { expect, test, type Page, type Route } from '@playwright/test'

function approveWorkflow() {
  return {
    id: 'wf-approve',
    name: '自我迭代PRO',
    description: '开发前澄清 + 计划',
    status: 'published',
    version: 1,
    updatedAt: '2026-08-10T12:00:00Z',
    needsRepo: false,
    nodes: [
      { id: 'in', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
      { id: 'ap', type: 'approve', label: '澄清', position: { x: 0, y: 0 }, config: {} },
      { id: 'out', type: 'output', label: '结束', position: { x: 0, y: 0 }, config: {} },
    ],
    edges: [
      { id: 'e1', source: 'in', target: 'ap' },
      { id: 'e2', source: 'ap', target: 'out' },
    ],
  }
}

type ReplyBody = { text?: string; images?: unknown[]; force?: boolean; annotations?: unknown[] }

async function mockHomeApis(page: Page, capture?: { replies: ReplyBody[]; titles: string[] }) {
  await page.route('**/api/workflows**', async (route: Route) => {
    const req = route.request()
    const url = req.url()
    if (req.method() === 'POST' && url.includes('/runs')) {
      const body = req.postDataJSON() as { title?: string } | null
      if (capture && body?.title) capture.titles.push(String(body.title))
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'run-home', status: 'queued' }),
      })
      return
    }
    if (req.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([approveWorkflow()]),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/runs/**', async (route: Route) => {
    const req = route.request()
    const url = req.url()
    if (req.method() === 'POST' && url.includes('/react/') && url.includes('/reply')) {
      const body = (req.postDataJSON() || {}) as ReplyBody
      capture?.replies.push(body)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'ok' }),
      })
      return
    }
    if (req.method() === 'GET' && url.includes('/runs/run-home')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'run-home',
          status: 'waiting_human',
          nodes: approveWorkflow().nodes,
          edges: approveWorkflow().edges,
          nodeRuns: { ap: { nodeId: 'ap', status: 'waiting_human' } },
          artifacts: [],
        }),
      })
      return
    }
    await route.continue()
  })

  await page.route('**/api/projects**', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ items: [], total: 0 }),
      })
      return
    }
    await route.continue()
  })
}

test.describe('首页 Chat 启动 Approve 流水线', () => {
  test('无项目记忆：空态引导至项目列表', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockHomeApis(page)
    await page.goto('/dashboard-home-chat.html')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('home-no-project')).toBeVisible()
    await page.getByTestId('dashboard-select-project').click()
    await expect(page.getByTestId('projects-page')).toBeVisible({ timeout: 5_000 })
  })

  test('发送第一句话：StartRun + ReactReply 后进入运行详情', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockHomeApis(page)
    await page.goto('/dashboard-home-chat.html?memory=1&projectId=proj-1')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('home-pipeline-card-wf-approve')).toContainText('自我迭代PRO')

    await page.getByTestId('home-composer-input').fill('把登录做清楚')
    await page.getByTestId('home-composer-send').click()

    await expect(page.getByTestId('run-detail-page')).toBeVisible({ timeout: 10_000 })
  })

  test('仅附件启动：默认标题附件启动并进入运行详情', async ({ page }) => {
    const capture = { replies: [] as ReplyBody[], titles: [] as string[] }
    await page.setViewportSize({ width: 1280, height: 800 })
    await mockHomeApis(page, capture)
    await page.goto('/dashboard-home-chat.html?memory=1&projectId=proj-1')
    await expect(page.getByTestId('dashboard-view')).toBeVisible({ timeout: 10_000 })

    const plus = page.getByTestId('home-composer-plus')
    await expect(plus).toBeEnabled()
    await expect(plus).toHaveAttribute('title', '添加附件')

    await page.getByTestId('home-attach-input').setInputFiles({
      name: 'brief.png',
      mimeType: 'image/png',
      buffer: Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
        'base64',
      ),
    })
    await expect(page.getByTestId('home-pending-attachments')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByTestId('home-draft-image-thumb')).toBeVisible()

    await page.getByTestId('home-composer-send').click()
    await expect(page.getByTestId('run-detail-page')).toBeVisible({ timeout: 10_000 })
    expect(capture.titles).toContain('附件启动')
    expect(capture.replies.length).toBeGreaterThan(0)
    const last = capture.replies[capture.replies.length - 1]
    expect(last.text ?? '').toBe('')
    expect(Array.isArray(last.images) && last.images.length).toBeTruthy()
  })
})

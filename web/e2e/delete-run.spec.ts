/**
 * Acceptance E2E for DeleteRun UI on RunDetailView (mocked API).
 * Harness page loads /runs/run-responsive-e2e via run-detail-real-main.ts.
 */
import { expect, test, type Page } from '@playwright/test'

const RUN_ID = 'run-responsive-e2e'
const WF_ID = 'wf-delete-e2e'

const nodes = [
  { id: 'input', type: 'input', label: '输入', position: { x: 0, y: 0 }, config: {} },
  { id: 'agent', type: 'agent', label: '执行', position: { x: 180, y: 0 }, config: {} },
]

function buildRun(status: string) {
  return {
    id: RUN_ID,
    workflowId: WF_ID,
    workflowName: '删除验收工作流',
    workflowVersion: 1,
    status,
    trigger: 'manual',
    startedAt: '2026-07-24T12:00:00Z',
    durationSec: 42,
    progress: status === 'completed' || status === 'failed' || status === 'cancelled' ? 1 : 0.4,
    branch: 'feature/delete-run',
    git: { pushed: false, branch: 'feature/delete-run' },
    nodeRuns: {
      input: {
        nodeId: 'input',
        status: 'completed',
        startedAt: '2026-07-24T12:00:00Z',
        durationSec: 1,
        varsSnapshot: {},
        outputs: {},
      },
      agent: {
        nodeId: 'agent',
        status: status === 'running' ? 'running' : status === 'waiting_human' ? 'waiting_human' : 'completed',
        startedAt: '2026-07-24T12:00:01Z',
        durationSec: 40,
        varsSnapshot: {},
        outputs: {},
      },
    },
    nodeExecutions: {},
    artifacts: [],
    trace: [],
    vars: [],
    nodes,
    edges: [{ id: 'e1', source: 'input', target: 'agent' }],
  }
}

async function openRunDetail(page: Page, status: string, onDelete?: () => void) {
  await page.setViewportSize({ width: 1280, height: 900 })

  await page.route('**/api/**', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const path = url.pathname
    const method = req.method()

    if (method === 'DELETE' && path === `/api/runs/${RUN_ID}`) {
      onDelete?.()
      await route.fulfill({ json: { status: 'deleted' } })
      return
    }

    if (path.includes('/events')) {
      await route.fulfill({ json: { events: [], nextCursor: '', hasMore: false, live: false } })
      return
    }
    if (path.includes('/sandbox-log')) {
      await route.fulfill({ json: { content: '', live: false, found: false } })
      return
    }
    if (path === `/api/runs/${RUN_ID}`) {
      await route.fulfill({ json: buildRun(status) })
      return
    }
    if (path === '/api/runs' || path.startsWith('/api/runs')) {
      await route.fulfill({ json: { items: [buildRun(status)], total: 1 } })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })

  await page.goto('/run-detail-real.html')
  await expect(page.getByTestId('run-detail-root')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('delete-run-btn')).toBeVisible()
}

test.describe('DeleteRun UI acceptance', () => {
  test('failed: delete enabled, confirm copy mentions permanent delete and not WorkflowDef', async ({
    page,
  }) => {
    await openRunDetail(page, 'failed')
    const btn = page.getByTestId('delete-run-btn')
    await expect(btn).toBeEnabled()
    await expect(page.getByTestId('delete-run-hint')).toHaveCount(0)
    await page.screenshot({ path: '/tmp/delete-run-failed-detail.png', fullPage: true })

    await btn.click()
    await expect(page.getByText('确认删除该次运行？')).toBeVisible()
    await expect(page.getByText(/永久删除该次运行及关联数据/)).toBeVisible()
    await expect(page.getByText(/不会删除工作流定义/)).toBeVisible()
    await page.screenshot({ path: '/tmp/delete-run-failed-confirm.png', fullPage: true })

    await page.getByRole('button', { name: '取消' }).click()
    await expect(page.getByText('确认删除该次运行？')).toHaveCount(0)
    await expect(page.getByTestId('run-detail-root')).toBeVisible()
  })

  test('completed: confirm delete calls DELETE and leaves detail view', async ({ page }) => {
    let deleteCalled = false
    await openRunDetail(page, 'completed', () => {
      deleteCalled = true
    })
    await expect(page.getByTestId('delete-run-btn')).toBeEnabled()
    await page.getByTestId('delete-run-btn').click()
    await page.screenshot({ path: '/tmp/delete-run-completed-confirm.png', fullPage: true })
    await page.getByTestId('confirm-delete-run-btn').click()
    await expect.poll(() => deleteCalled).toBe(true)
    await expect(page.getByTestId('run-detail-root')).toHaveCount(0)
    await page.screenshot({ path: '/tmp/delete-run-completed-after.png', fullPage: true })
  })

  test('running: delete disabled with active hint', async ({ page }) => {
    await openRunDetail(page, 'running')
    await expect(page.getByTestId('delete-run-btn')).toBeDisabled()
    await expect(page.getByTestId('delete-run-hint')).toContainText(/取消|等待/)
    await page.screenshot({ path: '/tmp/delete-run-running-disabled.png', fullPage: true })
  })

  test('cancelled: delete enabled, confirm leaves detail view', async ({ page }) => {
    let deleteCalled = false
    await openRunDetail(page, 'cancelled', () => {
      deleteCalled = true
    })
    const btn = page.getByTestId('delete-run-btn')
    await expect(btn).toBeVisible()
    await expect(btn).toBeEnabled()
    await expect(page.getByTestId('delete-run-hint')).toHaveCount(0)
    await page.screenshot({ path: '/tmp/delete-run-cancelled-enabled.png', fullPage: true })

    await btn.click()
    await expect(page.getByText('确认删除该次运行？')).toBeVisible()
    await page.getByTestId('confirm-delete-run-btn').click()
    await expect.poll(() => deleteCalled).toBe(true)
    await expect(page.getByTestId('run-detail-root')).toHaveCount(0)
    await page.screenshot({ path: '/tmp/delete-run-cancelled-after.png', fullPage: true })
  })

  test('queued: delete disabled with active hint', async ({ page }) => {
    await openRunDetail(page, 'queued')
    await expect(page.getByTestId('delete-run-btn')).toBeDisabled()
    await expect(page.getByTestId('delete-run-hint')).toContainText(/取消|等待/)
  })
})

/**
 * g6.2 / g6.3: completed deep link → output master-detail list / enlarge / Demo empty / mobile detail.
 */
import { test, expect, type Page } from '@playwright/test'

const nodes = [
  { id: 'start', type: 'input', label: '开始', position: { x: 0, y: 0 }, config: {} },
  { id: 'research', type: 'research', label: '代码调研', position: { x: 180, y: 0 }, config: {} },
  { id: 'end', type: 'output', label: '结束', position: { x: 360, y: 0 }, config: {} },
]

const outputCards = [
  {
    index: 1,
    template: 'research',
    title: '调研 · 代码调研',
    typeTag: 'Markdown',
    status: 'ok',
    markdown: '## 调研结论\n首张结果卡正文',
  },
  {
    index: 2,
    template: 'plan',
    title: '计划 · 制定计划',
    typeTag: 'Markdown',
    status: 'ok',
    markdown: '## 计划正文\n第二张',
  },
  {
    index: 3,
    template: 'failed',
    title: '测试 · 失败卡',
    typeTag: '来源失败',
    status: 'failed',
    errorReason: '上游测试节点失败',
  },
]

function mockRun(opts: { empty?: boolean; outputRan?: boolean } = {}) {
  const empty = !!opts.empty
  const outputRan = opts.outputRan !== false
  return {
    id: 'run-completed-e2e',
    workflowId: 'wf-completed-e2e',
    workflowName: '需求→调研→实现',
    workflowVersion: 1,
    status: 'completed',
    trigger: 'manual',
    startedAt: '2026-08-01T00:00:00Z',
    durationSec: 120,
    progress: 1,
    nodeRuns: {
      start: { nodeId: 'start', status: 'completed', startedAt: '2026-08-01T00:00:00Z', outputs: {} },
      research: {
        nodeId: 'research',
        status: 'completed',
        startedAt: '2026-08-01T00:01:00Z',
        outputs: {},
      },
      end: outputRan
        ? {
            nodeId: 'end',
            status: 'completed',
            startedAt: '2026-08-01T00:02:00Z',
            outputs: { outputCards: empty ? [] : outputCards },
          }
        : { nodeId: 'end', status: 'pending', outputs: {} },
    },
    nodeExecutions: {},
    artifacts: [],
    trace: [],
    vars: [],
    nodes,
    edges: [
      { id: 'e1', source: 'start', target: 'research' },
      { id: 'e2', source: 'research', target: 'end' },
    ],
  }
}

async function mockApi(page: Page, run: ReturnType<typeof mockRun>) {
  await page.route('**/api/runs/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/events')) {
      await route.fulfill({ json: { events: [], nextCursor: '', hasMore: false, live: false } })
      return
    }
    if (url.pathname.endsWith('/sandbox-log') || url.pathname.includes('/sandbox')) {
      await route.fulfill({ json: { content: '', live: false, found: false } })
      return
    }
    if (url.pathname.endsWith('/run-completed-e2e') || url.pathname.includes('/runs/run-completed-e2e')) {
      await route.fulfill({ json: run })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })
  await page.route('**/api/workflows/**', async (route) => {
    await route.fulfill({
      json: {
        id: 'wf-completed-e2e',
        name: '需求→调研→实现',
        status: 'published',
        version: 1,
        nodes,
        edges: [],
      },
    })
  })
}

async function gotoDeepLink(
  page: Page,
  opts: { width?: number; empty?: boolean; outputRan?: boolean; query?: string } = {},
) {
  await page.setViewportSize({ width: opts.width ?? 1280, height: 900 })
  await mockApi(page, mockRun({ empty: opts.empty, outputRan: opts.outputRan }))
  const qs = opts.query ?? 'node=end&tab=output'
  await page.goto(`/run-completed-output.html?${qs}`)
  await expect(page.getByTestId('run-detail-root')).toBeVisible({ timeout: 15_000 })
}

test.describe('completed 深链输出视图 (g6.2/g6.3)', () => {
  test('已登录深链：名称列表单选 + 放大模态 + 失败行 (g4.2)', async ({ page }) => {
    await gotoDeepLink(page)
    await expect(page.getByTestId('run-detail-right-panel')).toBeVisible()
    await expect(page.getByTestId('output-result-cards')).toBeVisible()
    await expect(page.getByTestId('output-result-list')).toBeVisible()
    await expect(page.getByTestId('output-result-list-header')).toContainText('产出')
    await expect(page.getByTestId('output-result-card-toggle-0')).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('output-result-card-body-0')).toContainText('首张结果卡正文')
    await expect(page.getByTestId('output-result-card-body-1')).toHaveCount(0)
    await expect(page.getByTestId('run-detail-right-panel')).toContainText('结束')
    await expect(page.getByTestId('run-detail-right-panel')).not.toContainText('打开原始文件')
    await expect(page.getByTestId('run-detail-right-panel')).not.toContainText('下载')
    await expect(page.getByTestId('run-detail-right-panel')).not.toContainText('窗口放大查看')

    await page.getByTestId('output-result-card-toggle-0').click()
    await expect(page.getByTestId('output-result-card-toggle-0')).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('output-result-card-body-0')).toContainText('首张结果卡正文')

    await page.getByTestId('output-result-enlarge').click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog).toContainText('首张结果卡正文')
    await expect(dialog).not.toContainText('打开原始文件')
    await expect(dialog).not.toContainText('下载')
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(page.getByTestId('output-result-card-toggle-0')).toHaveAttribute('aria-selected', 'true')

    await page.getByTestId('output-result-card-toggle-1').click()
    await expect(page.getByTestId('output-result-card-toggle-0')).toHaveAttribute('aria-selected', 'false')
    await expect(page.getByTestId('output-result-card-body-1')).toContainText('第二张')
    await expect(page.getByTestId('output-result-enlarge')).toBeVisible()

    await page.getByTestId('output-result-card-toggle-2').click()
    await expect(page.getByTestId('output-result-card-body-2')).toContainText('上游测试节点失败')
    await expect(page.getByTestId('output-result-list-kind-2')).toHaveText('失败')
    await expect(page.getByTestId('output-result-enlarge')).toHaveCount(0)
  })

  test('无结果卡：Demo 空态文案', async ({ page }) => {
    await gotoDeepLink(page, { empty: true, query: 'tab=output' })
    await expect(page.getByTestId('node-output-empty')).toBeVisible()
    await expect(page.getByTestId('node-output-empty')).toContainText('本次没有可预览的结果卡')
    await expect(page.getByTestId('node-output-empty')).toContainText('这不是加载失败')
    await expect(page.getByTestId('node-output-empty')).toContainText('Artifacts Tab')
    await expect(page.getByTestId('run-detail-right-panel')).not.toContainText('该节点尚未执行')
  })

  test('输出节点未跑过且 run=completed：仍空态而非尚未运行', async ({ page }) => {
    await gotoDeepLink(page, { outputRan: false, query: 'node=end&tab=output' })
    await expect(page.getByTestId('node-output-empty')).toBeVisible()
    await expect(page.getByTestId('node-output-empty')).toContainText('本次没有可预览的结果卡')
    await expect(page.getByTestId('run-detail-right-panel')).not.toContainText('该节点尚未执行')
  })

  test('移动端深链首屏为详情面板，可返回时间线', async ({ page }) => {
    await gotoDeepLink(page, { width: 390 })
    await expect(page.getByTestId('mobile-main-panel-tabs')).toBeVisible()
    await expect(page.getByTestId('run-detail-right-panel')).toBeVisible()
    await expect(page.getByTestId('output-result-cards')).toBeVisible()
    await expect(page.getByTestId('output-result-card-toggle-0')).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByTestId('run-timeline-pane')).toBeHidden()

    await page.getByTestId('mobile-back-to-timeline').click()
    await expect(page.getByTestId('run-timeline-pane')).toBeVisible()
    await expect(page.getByTestId('run-detail-right-panel')).toBeHidden()
    await page.getByTestId('mobile-panel-detail').click()
    await expect(page.getByTestId('run-detail-right-panel')).toBeVisible()
  })
})

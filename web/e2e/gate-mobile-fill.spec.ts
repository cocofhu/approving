import { test, expect } from '@playwright/test'

async function mockApi(
  page: import('@playwright/test').Page,
  issues: Array<Record<string, unknown>> = [],
) {
  await page.route('**/api/**', async (route) => {
    // Skip Vite module URLs like /@fs/.../src/lib/api/api.ts (pathname is not /api/...)
    if (!new URL(route.request().url()).pathname.startsWith('/api/')) {
      await route.continue()
      return
    }
    const url = new URL(route.request().url())
    if (url.pathname.includes('/preview-issues')) {
      await route.fulfill({ json: { issues } })
      return
    }
    if (url.pathname.includes('/primary-artifacts') || url.pathname.includes('/gate/')) {
      await route.fulfill({
        status: 400,
        json: { error: 'offline' },
      })
      return
    }
    if (url.pathname.includes('/artifacts/')) {
      await route.fulfill({
        json: {
          content: JSON.stringify({ summary: '上游需求', goals: ['g1'] }),
          etag: 'e1',
          updatedAt: '2026-07-18T00:00:00Z',
          sizeBytes: 32,
        },
      })
      return
    }
    await route.fulfill({ status: 404, json: { error: 'not mocked' } })
  })
}

// plan_coverage: g2.1 mobile-fill no drawer-height=340; g3.2 e2e stage min + drag resize (leaf ids only).
test.describe('Run 详情移动端 visual 定高预览', () => {
  // plan_coverage: g2.1 / g3.2 — adaptive drawer<340, stage>160, handle drag grows drawer
  test('390×844：n_open=0 确认并流转 + 取点，预览占满 stage', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockApi(page, [])

    await page.goto('/gate-mobile-fill.html')
    await expect(page.getByTestId('gate-mobile-fill-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('mobile-fill-remaining')).toBeVisible()
    await expect(page.getByTestId('review-shell')).toBeVisible()
    await expect(page.getByTestId('review-shell-drawer-handle')).toBeVisible()

    const preview = page.getByTestId('mobile-fill-preview')
    const drawer = page.getByTestId('review-shell-sidebar')
    const feedback = page.getByTestId('mobile-fill-feedback')
    await expect(preview).toBeVisible()
    await expect(drawer).toBeVisible()
    await expect(feedback).toBeVisible()

    const [previewBox, drawerBox, rootBox, feedbackBox] = await Promise.all([
      preview.boundingBox(),
      drawer.boundingBox(),
      page.getByTestId('gate-mobile-fill-panel').boundingBox(),
      feedback.boundingBox(),
    ])
    expect(previewBox && drawerBox && rootBox && feedbackBox).toBeTruthy()

    // Stage (preview) sits above the ReviewShell drawer; drawer is a sizable bottom panel.
    expect(previewBox!.y).toBeLessThan(drawerBox!.y)
    expect(previewBox!.height).toBeGreaterThan(160)
    // Adaptive default is lower than the old fixed 340px so preview stays readable.
    expect(drawerBox!.height).toBeLessThan(340)
    expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(rootBox!.y + rootBox!.height + 1)

    // Dragging the handle upward grows the drawer.
    const handle = page.getByTestId('review-shell-drawer-handle')
    const handleBox = await handle.boundingBox()
    expect(handleBox).toBeTruthy()
    const initialDrawerHeight = drawerBox!.height
    await page.mouse.move(handleBox!.x + handleBox!.width / 2, handleBox!.y + handleBox!.height / 2)
    await page.mouse.down()
    await page.mouse.move(handleBox!.x + handleBox!.width / 2, handleBox!.y - 80, { steps: 6 })
    await page.mouse.up()
    const draggedDrawerBox = await drawer.boundingBox()
    expect(draggedDrawerBox).toBeTruthy()
    expect(draggedDrawerBox!.height).toBeGreaterThan(initialDrawerHeight)

    // Feedback lives inside the sidebar/drawer (not under the stage preview).
    expect(feedbackBox!.y).toBeGreaterThanOrEqual(drawerBox!.y - 1)
    expect(feedbackBox!.y).toBeLessThan(drawerBox!.y + drawerBox!.height)

    // Review semantics: 确认并流转 visible; no 通过/打回 dual buttons.
    await expect(page.getByTestId('review-composer-pass')).toBeVisible()
    await expect(page.getByRole('button', { name: '确认并流转' })).toBeVisible()
    await expect(page.getByRole('button', { name: '通过并流转' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '打回修改' })).toHaveCount(0)
    await expect(page.getByTestId('review-record-issue')).toBeVisible()
    await expect(page.getByRole('button', { name: '记入意见' })).toBeVisible()
    // Hot session: send remains available for in-place revise.
    await expect(page.getByTestId('review-composer-send')).toBeVisible()
    await expect(page.getByRole('button', { name: '发送' })).toBeVisible()
    // Single main text input in the drawer.
    await expect(drawer.getByTestId('paragraph-input')).toHaveCount(1)
    // Narrow drawer must expose image attach — not text-only.
    const attach = drawer.getByTestId('paragraph-input-attach')
    await expect(attach).toBeVisible()

    // Unsubmitted draft alone must not restore 打回 wording.
    const fileInput = drawer.locator('input[type="file"]')
    await fileInput.setInputFiles({
      name: 'reject-only.png',
      mimeType: 'image/png',
      buffer: Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
        'base64',
      ),
    })
    await expect(page.getByRole('button', { name: '打回修改' })).toHaveCount(0)
    await expect(page.getByTestId('review-composer-pass')).toBeVisible()

    // Inspect toggle remains available inside the preview shell.
    await expect(page.getByTestId('html-preview-inspect-bar')).toBeVisible()
    await expect(page.getByRole('button', { name: '取点标注' })).toBeVisible()
  })

  test('390×844：n_open≥1 可继续发送，确认并流转禁用', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockApi(page, [
      {
        id: 'iss-open',
        runId: 'run-gate-fill',
        nodeId: 'hg-visual',
        body: '布局需调整',
        status: 'open',
        createdAt: '2026-07-18T00:01:00Z',
      },
    ])

    await page.goto('/gate-mobile-fill.html')
    await expect(page.getByTestId('gate-mobile-fill-root')).toBeVisible({ timeout: 10_000 })

    // Open issues: keep send (no 打回) + confirm disabled (not unmounted).
    const send = page.getByTestId('review-composer-send')
    const pass = page.getByTestId('review-composer-pass')
    await expect(send).toBeVisible()
    await expect(pass).toBeVisible()
    await expect(send).toContainText('发送')
    await expect(pass).toContainText('确认并流转')
    await expect(pass).toBeDisabled()
    await expect(send).toBeEnabled()
    await expect(page.getByRole('button', { name: /打回/ })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '通过并流转' })).toHaveCount(0)
  })
})

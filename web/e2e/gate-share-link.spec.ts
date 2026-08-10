import { test, expect } from '@playwright/test'

test.describe('human_gate 临时审批链接', () => {
  test('Inbox 复制临时链接打开管理面板且桌面按钮不高于花粒', async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: {
          writeText: async (text: string) => {
            ;(window as unknown as { __copied?: string }).__copied = text
          },
        },
      })
    })
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto('/gate-share-link.html?scene=inbox')
    await expect(page.getByTestId('gate-share-e2e-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gate-share-copy-btn')).toBeVisible()
    await expect(page.getByTestId('gate-share-status')).toContainText('尚未创建')
    const btnBox = await page.getByTestId('gate-share-copy-btn').boundingBox()
    const chipBox = await page.getByTestId('gate-share-status').boundingBox()
    const wrapBox = await page.getByTestId('gate-share-hit-wrap').boundingBox()
    expect(btnBox && chipBox && wrapBox).toBeTruthy()
    expect(btnBox!.height).toBeLessThanOrEqual(chipBox!.height + 8)
    // desktop: click box == visible box, not forced to 44px
    expect(Math.abs(wrapBox!.height - btnBox!.height)).toBeLessThanOrEqual(2)
    expect(wrapBox!.height).toBeLessThan(44)
    const shareTb = page.getByTestId('html-preview-share-link').first()
    const inspectTb = page.getByTestId('html-preview-inspect-toggle').first()
    await expect(shareTb).toBeVisible()
    const shareH = (await shareTb.boundingBox())?.height ?? 0
    const inspectH = (await inspectTb.boundingBox())?.height ?? 0
    expect(shareH).toBeGreaterThan(0)
    expect(Math.abs(shareH - inspectH)).toBeLessThanOrEqual(4)
    await page.getByTestId('gate-share-copy-btn').click()
    await expect(page.getByTestId('gate-share-panel-body')).toBeVisible()
    await expect(page.getByTestId('gate-share-panel-body')).toContainText('信任')
    await expect(page.getByTestId('gate-share-panel-body')).toContainText('审批工作台')
    await expect(page.getByTestId('gate-share-panel-body')).toContainText('可取点')
    await page.getByTestId('gate-share-create').click()
    await expect(page.getByTestId('gate-share-url')).toBeVisible()
    await expect(page.getByTestId('gate-share-url')).toHaveValue(/#t=••••/)
    const copied = await page.evaluate(() => (window as unknown as { __copied?: string }).__copied || '')
    expect(copied).toContain('/public/gate-approvals#t=')
    expect(copied).toMatch(/#t=[0-9a-f]{64}$/)
  })

  test('未登录外部页可确认', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/gate-share-link.html?scene=public')
    await expect(page.getByTestId('public-gate-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.locator('html')).not.toHaveClass(/light/)
    await expect(page.getByTestId('public-gate-badge')).toHaveText('外部一次决策')
    await expect(page.getByTestId('review-shell')).toBeVisible()
    await expect(page.getByTestId('public-gate-product-label')).toContainText('视觉网页产物')
    await expect(page.getByTestId('public-gate-footer')).toContainText('上游上下文')
    await expect(page.getByTestId('public-gate-upstream-enlarge')).toBeVisible()
    await expect(page.getByTestId('public-gate-root')).not.toContainText('run-e2e')
    await expect(page.getByTestId('public-gate-root')).not.toContainText('请确认本次交付')
    await expect(page.getByTestId('public-gate-confirm')).toHaveText('确认并流转')
    await expect(page.getByTestId('public-gate-reject')).toHaveText('驳回')
    await expect(page.getByTestId('clarify-confirm-flow')).toHaveCount(0)
    await page.getByTestId('public-gate-name').fill('Jordan')
    await page.getByTestId('public-gate-comment').fill('可以流转')
    await page.getByTestId('public-gate-confirm').click()
    await expect(page.getByTestId('public-gate-done')).toContainText('已确认')
    await expect(page.getByTestId('public-gate-confirm')).toHaveCount(0)
    await expect(page.getByTestId('public-gate-reject')).toHaveCount(0)
  })

  test('未登录外部页驳回需意见', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/gate-share-link.html?scene=public')
    await expect(page.getByTestId('public-gate-reject')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('public-gate-reject')).toHaveText('驳回')
    await page.getByTestId('public-gate-reject').click()
    await expect(page.getByText('请填写姓名与意见后再提交')).toBeVisible()
    await page.getByTestId('public-gate-name').fill('Jordan')
    await page.getByTestId('public-gate-comment').fill('需要修改文案')
    await page.getByTestId('public-gate-reject').click()
    await expect(page.getByTestId('public-gate-done')).toContainText('已驳回')
  })

  test('移动端 Inbox 复制临时链接贴齐花粒且 hit 区 ≥44', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/gate-share-link.html?scene=inbox')
    await expect(page.getByTestId('gate-share-e2e-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gate-share-copy-btn')).toBeVisible()
    await expect(page.getByTestId('gate-share-status')).toContainText('尚未创建')
    const btnBox = await page.getByTestId('gate-share-copy-btn').boundingBox()
    const chipBox = await page.getByTestId('gate-share-status').boundingBox()
    const wrapBox = await page.getByTestId('gate-share-hit-wrap').boundingBox()
    expect(btnBox && chipBox && wrapBox).toBeTruthy()
    expect(btnBox!.height).toBeLessThanOrEqual(chipBox!.height + 8)
    expect(wrapBox!.width).toBeGreaterThanOrEqual(44)
    expect(wrapBox!.height).toBeGreaterThanOrEqual(44)
    await page.getByTestId('gate-share-status').click()
    await expect(page.getByTestId('gate-share-panel-body')).toHaveCount(0)
    await page.getByTestId('gate-share-copy-btn').click()
    await expect(page.getByTestId('gate-share-panel-body')).toBeVisible()
  })

  test('待复审 Inbox 可创建并复制临时链接', async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: {
          writeText: async (text: string) => {
            ;(window as unknown as { __copied?: string }).__copied = text
          },
        },
      })
    })
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto('/gate-share-link.html?scene=inbox-review')
    await expect(page.getByTestId('gate-share-copy-btn')).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('gate-share-copy-btn').click()
    await expect(page.getByTestId('gate-share-panel-body')).toBeVisible()
    await page.getByTestId('gate-share-create').click()
    await expect(page.getByTestId('gate-share-url')).toBeVisible()
    const copied = await page.evaluate(() => (window as unknown as { __copied?: string }).__copied || '')
    expect(copied).toContain('/public/gate-approvals#t=')
  })

  test('未登录复审页三区可发送并底栏确认', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto('/gate-share-link.html?scene=public-review')
    await expect(page.getByTestId('public-gate-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('public-gate-root')).toContainText('外部复审')
    await expect(page.getByTestId('review-shell')).toBeVisible()
    await expect(page.getByTestId('public-gate-sidebar')).toContainText('Agent交互')
    await expect(page.getByTestId('clarify-input')).toBeVisible()
    await expect(page.getByTestId('public-gate-confirm')).toBeVisible()
    await expect(page.getByTestId('public-gate-footer')).toContainText('确认并流转')
    await expect(page.getByTestId('clarify-confirm-flow')).toHaveCount(0)
    await expect(page.getByTestId('public-gate-reject')).toHaveCount(0)
    await expect(page.getByTestId('public-gate-name')).toHaveCount(0)
    await page.getByTestId('clarify-input').fill('请改标题')
    await page.getByTestId('clarify-send-icon').click()
    await expect(page.getByTestId('clarify-review-queue')).toBeVisible()
    await expect(page.getByTestId('clarify-review-queue')).toContainText('请改标题')
    await expect(page.getByTestId('public-gate-done')).toHaveCount(0)
    await expect(page.getByTestId('review-shell')).toBeVisible()
    await page.evaluate(() => {
      ;(window as unknown as { __idleReview?: () => void }).__idleReview?.()
      window.dispatchEvent(new HashChangeEvent('hashchange'))
    })
    await expect(page.getByTestId('public-gate-confirm')).toBeVisible()
    await page.getByTestId('public-gate-confirm').click()
    await expect(page.getByTestId('public-gate-done')).toContainText('已确认')
    await expect(page.getByTestId('public-gate-confirm')).toHaveCount(0)
  })
})

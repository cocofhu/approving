import { test, expect } from '@playwright/test'

test.describe('GatesInbox clarify 产物台', () => {
  test('research 预览、多产物切换与 react 澄清产物', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })

    await page.goto('/clarify-inbox-product.html?scenario=research')
    await expect(page.getByTestId('clarify-inbox-product-root')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('clarify-product-stage')).toBeVisible()
    await expect(page.getByTestId('clarify-product-tabs')).toBeVisible()
    await expect(page.getByText('调研结论摘要：澄清产物台应对齐 Gate 载荷。')).toBeVisible()

    await page.getByTestId('clarify-product-tab-plan_1').click()
    await expect(page.getByTestId('clarify-product-panel').getByText('实施计划', { exact: true })).toBeVisible()
    await expect(page.getByText('修复 clarify 空白')).toBeVisible()

    await page.goto('/clarify-inbox-product.html?scenario=react')
    await expect(page.getByTestId('clarify-product-stage')).toBeVisible()
    await expect(page.getByText('在审批箱澄清阶段展示结构化产物。')).toBeVisible()
    await expect(page.getByTestId('clarify-product-tabs')).toHaveCount(0)
  })

  test('三态空态、重试恢复与加载中不闪尚未执行', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })

    await page.goto('/clarify-inbox-product.html?scenario=pending')
    const pendingEmpty = page.getByTestId('clarify-product-empty-pending')
    await expect(pendingEmpty).toBeVisible({ timeout: 10_000 })
    await expect(pendingEmpty.getByText('节点尚未执行', { exact: true })).toBeVisible()

    await page.goto('/clarify-inbox-product.html?scenario=executedEmpty')
    const executedEmpty = page.getByTestId('clarify-product-empty-executedEmpty')
    await expect(executedEmpty).toBeVisible()
    await expect(executedEmpty.getByText('已执行但暂无产物', { exact: true })).toBeVisible()

    await page.goto('/clarify-inbox-product.html?scenario=loadFailed')
    const loadFailed = page.getByTestId('clarify-product-empty-loadFailed')
    await expect(loadFailed).toBeVisible()
    await expect(loadFailed.getByText('无法加载产物', { exact: true })).toBeVisible()
    await expect(page.getByText('请点击重试重新加载审批上下文')).toBeVisible()
    await page.getByTestId('clarify-product-retry').click()
    await expect(page.getByText('调研结论摘要：澄清产物台应对齐 Gate 载荷。')).toBeVisible({
      timeout: 10_000,
    })

    await page.goto('/clarify-inbox-product.html?scenario=loading')
    // While loading, final empty-state copy must not flash.
    await expect(page.getByTestId('clarify-product-empty-pending')).toHaveCount(0)
    await expect(page.getByText('加载运行详情…')).toBeVisible({ timeout: 5_000 })
    await expect(page.getByText('调研结论摘要：澄清产物台应对齐 Gate 载荷。')).toBeVisible({
      timeout: 10_000,
    })
  })
})

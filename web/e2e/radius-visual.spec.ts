import { expect, test } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import fs from 'node:fs'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const shotDir = path.join(__dirname, '..', '..', '..', 'test-screenshots')

function parseRadius(value: string): number {
  return parseFloat(String(value).split(' ')[0] || '0')
}

test.describe('unified radius visual acceptance', () => {
  test.beforeAll(() => {
    fs.mkdirSync(shotDir, { recursive: true })
  })

  test('role radii + light sidebar corner clean', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto('/radius-visual.html')
    await expect(page.getByTestId('radius-visual-root')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('app-sidebar-card')).toBeVisible()

    const measured = await page.evaluate(() => {
      const pick = (sel: string) => {
        const el = document.querySelector(sel) as HTMLElement | null
        if (!el) return null
        const cs = getComputedStyle(el)
        return {
          borderRadius: cs.borderRadius,
          overflow: cs.overflow,
          boxShadow: cs.boxShadow,
        }
      }
      const aside = document.querySelector('aside.app-desktop-sidebar, aside[class*="overflow"]') as HTMLElement | null
      return {
        control: pick('[data-testid="sample-control"]'),
        input: pick('[data-testid="sample-input"]'),
        navPill: pick('[data-testid="sample-nav-pill"]'),
        card: pick('[data-testid="sample-card"]'),
        modal: pick('[data-testid="sample-modal"]'),
        avatar: pick('[data-testid="sample-avatar"]'),
        sidebarCard: pick('[data-testid="app-sidebar-card"]'),
        asideOverflow: aside ? getComputedStyle(aside).overflow : null,
        asideClasses: aside?.className || '',
        hasGlobalZeroImportant: [...document.styleSheets].some((sheet) => {
          try {
            return [...sheet.cssRules].some((rule) => {
              const text = rule.cssText || ''
              return (
                text.includes('border-radius: 0px !important') ||
                text.includes('border-radius: 0 !important')
              ) && (text.startsWith('*') || text.includes('*,'))
            })
          } catch {
            return false
          }
        }),
      }
    })

    expect(measured.hasGlobalZeroImportant).toBe(false)
    expect(parseRadius(measured.control!.borderRadius)).toBeGreaterThanOrEqual(7)
    expect(parseRadius(measured.control!.borderRadius)).toBeLessThanOrEqual(9)
    expect(parseRadius(measured.input!.borderRadius)).toBeGreaterThanOrEqual(7)
    expect(parseRadius(measured.input!.borderRadius)).toBeLessThanOrEqual(9)
    expect(parseRadius(measured.navPill!.borderRadius)).toBeGreaterThanOrEqual(9)
    expect(parseRadius(measured.navPill!.borderRadius)).toBeLessThanOrEqual(11)
    expect(parseRadius(measured.card!.borderRadius)).toBeGreaterThanOrEqual(11)
    expect(parseRadius(measured.card!.borderRadius)).toBeLessThanOrEqual(13)
    expect(parseRadius(measured.modal!.borderRadius)).toBeGreaterThanOrEqual(15)
    expect(parseRadius(measured.modal!.borderRadius)).toBeLessThanOrEqual(17)
    expect(parseRadius(measured.avatar!.borderRadius)).toBeGreaterThanOrEqual(999)
    expect(parseRadius(measured.sidebarCard!.borderRadius)).toBeGreaterThanOrEqual(15)
    expect(parseRadius(measured.sidebarCard!.borderRadius)).toBeLessThanOrEqual(17)
    expect(measured.sidebarCard!.overflow).toMatch(/hidden|auto/)
    expect(measured.asideOverflow).toMatch(/visible/)

    // Corner probe: sample pixels just outside the card's bottom-right rounded edge.
    // Gray triangle would appear if square parent clips a shadowed rounded card.
    const cornerProbe = await page.evaluate(() => {
      const card = document.querySelector('[data-testid="app-sidebar-card"]') as HTMLElement
      const root = document.querySelector('[data-testid="radius-visual-root"]') as HTMLElement
      const rect = card.getBoundingClientRect()
      const canvas = document.createElement('canvas')
      canvas.width = 1
      canvas.height = 1
      // Use html2canvas-less approach: read background from computed root + elementFromPoint
      const samples: Array<{ x: number; y: number; tag: string; bg: string; color: string }> = []
      const points = [
        { x: rect.right - 2, y: rect.bottom - 2, tag: 'inside-corner' },
        { x: rect.right + 2, y: rect.bottom + 2, tag: 'outside-corner' },
        { x: rect.right - 1, y: rect.bottom + 3, tag: 'below-corner' },
        { x: rect.right + 3, y: rect.bottom - 1, tag: 'right-of-corner' },
      ]
      for (const p of points) {
        const el = document.elementFromPoint(p.x, p.y) as HTMLElement | null
        const cs = el ? getComputedStyle(el) : null
        samples.push({
          x: p.x,
          y: p.y,
          tag: p.tag,
          bg: cs?.backgroundColor || 'none',
          color: el?.className?.toString?.() || el?.tagName || 'null',
        })
      }
      return {
        samples,
        rootBg: getComputedStyle(root).backgroundColor,
        cardBg: getComputedStyle(card).backgroundColor,
      }
    })

    // Outside corner samples must hit the dotted canvas root (or body), not a gray clipped remnant layer.
    for (const s of cornerProbe.samples.filter((x) => x.tag !== 'inside-corner')) {
      expect(s.color).not.toMatch(/app-sidebar-card/)
    }

    await page.screenshot({
      path: path.join(shotDir, 'radius-light-sidebar.png'),
      fullPage: false,
    })

    // Dark theme same radii
    await page.getByTestId('sample-control').click()
    await page.waitForTimeout(200)
    const darkRadii = await page.evaluate(() => {
      const r = (sel: string) => getComputedStyle(document.querySelector(sel) as HTMLElement).borderRadius
      return {
        control: r('[data-testid="sample-control"]'),
        card: r('[data-testid="sample-card"]'),
        modal: r('[data-testid="sample-modal"]'),
        sidebar: r('[data-testid="app-sidebar-card"]'),
        isLight: document.documentElement.classList.contains('light'),
      }
    })
    expect(darkRadii.isLight).toBe(false)
    expect(parseRadius(darkRadii.control)).toBeGreaterThanOrEqual(7)
    expect(parseRadius(darkRadii.control)).toBeLessThanOrEqual(9)
    expect(parseRadius(darkRadii.card)).toBeGreaterThanOrEqual(11)
    expect(parseRadius(darkRadii.card)).toBeLessThanOrEqual(13)
    expect(parseRadius(darkRadii.modal)).toBeGreaterThanOrEqual(15)
    expect(parseRadius(darkRadii.sidebar)).toBeGreaterThanOrEqual(15)

    await page.screenshot({
      path: path.join(shotDir, 'radius-dark-sidebar.png'),
      fullPage: false,
    })

    // Zoom logout area for gray-triangle check (light)
    await page.getByTestId('sample-control').click()
    await page.waitForTimeout(200)
    const logout = page.locator('[data-testid="app-sidebar-card"]').locator('text=登出').first()
    if (await logout.count()) {
      await logout.scrollIntoViewIfNeeded()
    }
    const box = await page.getByTestId('app-sidebar-card').boundingBox()
    if (box) {
      await page.screenshot({
        path: path.join(shotDir, 'radius-light-sidebar-corner.png'),
        clip: {
          x: Math.max(0, box.x + box.width - 80),
          y: Math.max(0, box.y + box.height - 80),
          width: 100,
          height: 100,
        },
      })
    }
  })
})

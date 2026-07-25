// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'RunListView.vue'), 'utf8')

describe('RunListView cancel run', () => {
  it('exposes inline cancel for queued, running, and waiting_human', () => {
    expect(src).toMatch(/data-testid="cancel-run-btn"/)
    expect(src).toMatch(/data-testid="confirm-cancel-run-btn"/)
    expect(src).toMatch(/function canCancelRun\(/)
    expect(src).toMatch(/r\.status === 'queued' \|\| r\.status === 'running' \|\| r\.status === 'waiting_human'/)
    expect(src).toMatch(/common\.table\.actions/)
    expect(src).toMatch(/v-if="canCancelRun\(r\)"/)
  })

  it('stops row navigation when clicking cancel and confirms before POST', () => {
    expect(src).toMatch(/@click\.stop/)
    expect(src).toMatch(/openCancelConfirm\(r\)/)
    const openStart = src.indexOf('function openCancelConfirm(r: Run)')
    const openEnd = src.indexOf('\nfunction closeCancelConfirm(', openStart)
    const openFn = src.slice(openStart, openEnd)
    expect(openFn).toContain('cancelTarget.value = r')
    expect(openFn).not.toContain('api.cancelRun')

    const confirmStart = src.indexOf('async function confirmCancelRun()')
    const confirmEnd = src.indexOf('\nfunction runIdShort(', confirmStart)
    const confirmFn = src.slice(confirmStart, confirmEnd > 0 ? confirmEnd : undefined)
    expect(confirmFn).toContain('api.cancelRun(target.id)')
    expect(confirmFn).toContain("toast.success(t('pages.runDetail.cancelSuccess'))")
    expect(confirmFn).toContain('await load()')
  })

  it('keeps cancel confirm copy distinct from delete and supports mobile cards', () => {
    expect(src).toMatch(/pages\.runDetail\.cancelWarning/)
    expect(src).toMatch(/pages\.runList\.cancelConfirm/)
    expect(src).toMatch(/role="button"/)
    expect(src).toMatch(/@click\.stop @keydown\.stop/)
  })
})

describe('RunListView sorting', () => {
  it('exposes desktop sortable headers for start time and priority only', () => {
    expect(src).toMatch(/applySortClick\('started_at'\)/)
    expect(src).toMatch(/applySortClick\('priority'\)/)
    expect(src).toMatch(/role="columnheader"/)
    expect(src).toMatch(/aria-sort/)
    expect(src).toMatch(/th\.sortable/)
    expect(src).toMatch(/sort-icon/)
    // No product "restore default" control; sort UI is desktop-table only.
    expect(src).not.toMatch(/恢复默认|restoreDefault|clearSort/)
    expect(src).toMatch(/<!-- Desktop table -->[\s\S]*applySortClick\('started_at'\)/)
    expect(src).toMatch(/<!-- Mobile card list -->[\s\S]*?<!-- Desktop table -->/)
    const mobileBlock = src.slice(
      src.indexOf('<!-- Mobile card list -->'),
      src.indexOf('<!-- Desktop table -->'),
    )
    expect(mobileBlock).not.toContain('applySortClick')
    expect(mobileBlock).not.toContain('sort-icon')
  })

  it('syncs whitelist sort to URL and listParams; strips illegal pairs', () => {
    expect(src).toMatch(/parseRunSort/)
    expect(src).toMatch(/sort: sort\.sort, order: sort\.order/)
    expect(src).toMatch(/delete query\.sort/)
    expect(src).toMatch(/delete query\.order/)
    expect(src).toMatch(/router\.replace\(\{ query \}\)/)
    expect(src).toMatch(/started_at: 'desc'/)
    expect(src).toMatch(/priority: 'desc'/)
  })
})

// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'RunListView.vue'), 'utf8')

describe('RunListView click UX (loading / nav / prefetch)', () => {
  it('keeps table-loading opacity without pointer-events:none blocking the list', () => {
    const blockStart = src.indexOf('.table-loading {')
    expect(blockStart).toBeGreaterThanOrEqual(0)
    const blockEnd = src.indexOf('}', blockStart)
    const block = src.slice(blockStart, blockEnd + 1)
    expect(block).toMatch(/opacity:\s*0\.55/)
    expect(block).not.toMatch(/pointer-events:\s*none/)
    // Entire source must not reintroduce list-level pointer-events:none under table-loading
    expect(src).not.toMatch(/\.table-loading\s*\{[^}]*pointer-events:\s*none/)
  })

  it('uses RouterLink / to for run rows and cards', () => {
    expect(src).toMatch(/<RouterLink/)
    expect(src).toMatch(/:to="runHref\(r\.id\)"/)
    expect(src).toMatch(/function runHref\(id: string\)/)
    expect(src).toMatch(/return '\/runs\/' \+ id/)
    // Mobile card is a real link; desktop uses custom slot + navigate on tr
    expect(src).toMatch(/custom\s*\n\s*v-slot="\{ navigate, href \}"/)
    expect(src).toMatch(/@click="navigate"/)
  })

  it('prefetches RunDetail chunk on hover / touchstart / pointerdown', () => {
    expect(src).toMatch(/function prefetchRunDetail\(/)
    expect(src).toMatch(/import\('@\/views\/RunDetailView\.vue'\)/)
    expect(src).toMatch(/@mouseenter="prefetchRunDetail"/)
    expect(src).toMatch(/@touchstart\.passive="prefetchRunDetail"/)
    expect(src).toMatch(/@pointerdown="prefetchRunDetail"/)
  })

  it('skips poll assignment when list fingerprint is unchanged', () => {
    expect(src).toMatch(/function runsListUnchanged\(/)
    expect(src).toMatch(/function runListFingerprint\(/)
    expect(src).toMatch(/if \(!runsListUnchanged\(/)
    expect(src).toMatch(/setInterval\(load, 3000\)/)
  })
})

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
    const confirmEnd = src.indexOf('\nfunction openDeleteConfirm(', confirmStart)
    const confirmFn = src.slice(confirmStart, confirmEnd > 0 ? confirmEnd : undefined)
    expect(confirmFn).toContain('api.cancelRun(target.id)')
    expect(confirmFn).toContain("toast.success(t('pages.runDetail.cancelSuccess'))")
    expect(confirmFn).toContain('await load()')
  })

  it('keeps cancel confirm copy distinct from delete and supports mobile cards', () => {
    expect(src).toMatch(/pages\.runDetail\.cancelWarning/)
    expect(src).toMatch(/pages\.runList\.cancelConfirm/)
    // Mobile cards navigate via RouterLink (not role=button + openRun)
    expect(src).toMatch(/<!-- Mobile card list -->[\s\S]*?<RouterLink/)
    expect(src).toMatch(/@click\.stop @keydown\.stop/)
  })
})

describe('RunListView delete run and ops column', () => {
  it('adds canDeleteRun mutually exclusive with canCancelRun', () => {
    expect(src).toMatch(/function canDeleteRun\(/)
    expect(src).toMatch(
      /r\.status === 'completed' \|\| r\.status === 'failed' \|\| r\.status === 'cancelled'/,
    )
    expect(src).toMatch(/v-else-if="canDeleteRun\(r\)"/)
    expect(src).toMatch(/data-testid="delete-run-btn"/)
    expect(src).toMatch(/data-testid="run-ops-placeholder"/)
    // cancel and delete sets must not overlap in predicates
    const cancelFn = src.slice(
      src.indexOf('function canCancelRun(r: Run)'),
      src.indexOf('function canDeleteRun(r: Run)'),
    )
    const deleteFn = src.slice(
      src.indexOf('function canDeleteRun(r: Run)'),
      src.indexOf('function prefetchRunDetail('),
    )
    expect(cancelFn).not.toMatch(/completed|failed/)
    expect(deleteFn).not.toMatch(/queued|running|waiting_human/)
  })

  it('renders text danger buttons without icons and grey dash placeholder', () => {
    // List ops cancel/delete must be text-only; confirm modals may still use icons.
    const cancelBtnBlocks = [...src.matchAll(/data-testid="cancel-run-btn"[\s\S]*?<\/AppButton>/g)].map((m) => m[0])
    const deleteBtnBlocks = [...src.matchAll(/data-testid="delete-run-btn"[\s\S]*?<\/AppButton>/g)].map((m) => m[0])
    expect(cancelBtnBlocks.length).toBeGreaterThanOrEqual(2)
    expect(deleteBtnBlocks.length).toBeGreaterThanOrEqual(2)
    for (const block of [...cancelBtnBlocks, ...deleteBtnBlocks]) {
      expect(block).not.toMatch(/\bicon=/)
    }
    expect(src).toMatch(/data-testid="run-ops-placeholder"/)
    expect(src).toContain('—')
    expect(src).toMatch(/common\.buttons\.delete/)
  })

  it('opens delete confirm before DELETE and stays on list after success', () => {
    expect(src).toMatch(/openDeleteConfirm\(r\)/)
    expect(src).toMatch(/data-testid="confirm-delete-run-btn"/)
    expect(src).toMatch(/pages\.runDetail\.deleteWarning/)
    expect(src).toMatch(/pages\.runList\.deleteConfirm/)
    expect(src).toMatch(/pages\.runList\.deleteSuccess/)

    const openStart = src.indexOf('function openDeleteConfirm(r: Run)')
    const openEnd = src.indexOf('\nfunction closeDeleteConfirm(', openStart)
    const openFn = src.slice(openStart, openEnd)
    expect(openFn).toContain('deleteTarget.value = r')
    expect(openFn).not.toContain('api.deleteRun')

    const confirmStart = src.indexOf('async function confirmDeleteRun()')
    const confirmEnd = src.indexOf('\nwatch(statusFilterOpen', confirmStart)
    const confirmFn = src.slice(confirmStart, confirmEnd > 0 ? confirmEnd : undefined)
    expect(confirmFn).toContain('api.deleteRun(target.id)')
    expect(confirmFn).toContain("toast.success(t('pages.runList.deleteSuccess'))")
    expect(confirmFn).toContain('await load()')
    expect(confirmFn).not.toContain('router.push')
    expect(confirmFn).toContain('deletingRun.value = true')
    expect(confirmFn).toContain('mapDeleteRunError')
  })

  it('stops propagation on ops area and placeholder so row does not open detail', () => {
    expect(src).toMatch(/data-testid="run-ops"/)
    expect(src).toMatch(/data-testid="run-ops-placeholder"/)
    // desktop ops cell and mobile ops wrapper both stop click
    expect(src.match(/data-testid="run-ops"[^>]*@click\.stop/g)?.length).toBeGreaterThanOrEqual(2)
  })

  it('keeps cancel confirm flow and does not drop cancel modal', () => {
    expect(src).toMatch(/openCancelConfirm\(r\)/)
    expect(src).toMatch(/confirmCancelRun/)
    expect(src).toMatch(/data-testid="confirm-cancel-run-btn"/)
    expect(src).toMatch(/pages\.runDetail\.cancelTitle/)
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

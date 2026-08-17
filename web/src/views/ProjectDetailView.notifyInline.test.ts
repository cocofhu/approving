// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewsDir = dirname(fileURLToPath(import.meta.url))
const localesDir = join(viewsDir, '../locales')
const detailSrc = readFileSync(join(viewsDir, 'ProjectDetailView.vue'), 'utf8')
const enPages = JSON.parse(readFileSync(join(localesDir, 'en/pages.json'), 'utf8')) as {
  pages: { projectDetail: { notify: Record<string, string> } }
}
const zhPages = JSON.parse(readFileSync(join(localesDir, 'zh-CN/pages.json'), 'utf8')) as {
  pages: { projectDetail: { notify: Record<string, string> } }
}

function mobileWorkflowsBlock(): string {
  const start = detailSrc.indexOf('<!-- Mobile card list:')
  const end = detailSrc.indexOf('<!-- Desktop table -->')
  expect(start, 'missing mobile workflows card list').toBeGreaterThanOrEqual(0)
  expect(end, 'missing desktop table marker').toBeGreaterThan(start)
  return detailSrc.slice(start, end)
}

function desktopNotifyCell(): string {
  const start = detailSrc.indexOf('data-testid="wf-notify-cell"')
  expect(start, 'missing wf-notify-cell').toBeGreaterThanOrEqual(0)
  // Cell ends before the next Updated / actions column content after the segment.
  const end = detailSrc.indexOf('data-testid="wf-updated"', start)
  const fallbackEnd = detailSrc.indexOf('</td>', detailSrc.indexOf('modeCustom', start))
  const sliceEnd = end > start ? end : fallbackEnd + 5
  return detailSrc.slice(start, sliceEnd)
}

describe('Notify policy Inherit i18n (g1.1 / g1.2)', () => {
  it('en modeInherit is Inherit and hint no longer says Follow project', () => {
    const notify = enPages.pages.projectDetail.notify
    expect(notify.modeInherit).toBe('Inherit')
    expect(notify.defaultEventsHint).toContain('Inherit')
    expect(notify.defaultEventsHint).not.toMatch(/Follow project/i)
    expect(JSON.stringify(enPages)).not.toMatch(/Follow project/)
  })

  it('zh-CN modeInherit stays 跟随项目', () => {
    const notify = zhPages.pages.projectDetail.notify
    expect(notify.modeInherit).toBe('跟随项目')
    expect(notify.defaultEventsHint).toContain('跟随项目')
  })
})

describe('ProjectDetailView desktop wf-notify-cell (g2.1 / g3.1)', () => {
  it('forces Off/Inherit/Custom buttons to single-line with nowrap', () => {
    const cell = desktopNotifyCell()
    expect(cell.match(/shrink-0 whitespace-nowrap/g)?.length).toBe(3)
  })
})

describe('ProjectDetailView mobile wf-notify-inline (g2.2 / g3.1)', () => {
  it('puts Notify policy on its own full-width row, not sharing flex-1 with Run/Favorite', () => {
    const mobile = mobileWorkflowsBlock()
    expect(mobile).toMatch(/data-testid="wf-notify-inline"/)
    expect(mobile).not.toMatch(/flex min-w-0 flex-1 flex-col gap-1\.5"[\s\S]*data-testid="wf-notify-inline"/)
    expect(mobile).toMatch(
      /class="flex min-w-0 w-full flex-col gap-1\.5"[\s\S]*data-testid="wf-notify-inline"/,
    )
    const notifyStart = mobile.indexOf('data-testid="wf-notify-inline"')
    const menuStart = mobile.indexOf('data-wf-menu')
    expect(notifyStart).toBeGreaterThanOrEqual(0)
    expect(menuStart).toBeGreaterThan(notifyStart)
  })

  it('does not clip the nowrap segment with overflow-hidden; allows horizontal scroll instead', () => {
    const mobile = mobileWorkflowsBlock()
    const notifyStart = mobile.indexOf('data-testid="wf-notify-inline"')
    const menuStart = mobile.indexOf('data-wf-menu')
    const notify = mobile.slice(notifyStart, menuStart)
    expect(notify).toMatch(/max-w-full overflow-x-auto/)
    expect(notify).toMatch(/inline-flex w-max max-w-none/)
    expect(notify).not.toMatch(/overflow-hidden/)
    expect(notify.match(/shrink-0 whitespace-nowrap/g)?.length).toBe(3)
  })
})

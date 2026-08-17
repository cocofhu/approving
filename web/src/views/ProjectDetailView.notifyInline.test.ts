// @vitest-environment node
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewsDir = dirname(fileURLToPath(import.meta.url))
const detailSrc = readFileSync(join(viewsDir, 'ProjectDetailView.vue'), 'utf8')

function mobileWorkflowsBlock(): string {
  const start = detailSrc.indexOf('<!-- Mobile card list:')
  const end = detailSrc.indexOf('<!-- Desktop table -->')
  expect(start, 'missing mobile workflows card list').toBeGreaterThanOrEqual(0)
  expect(end, 'missing desktop table marker').toBeGreaterThan(start)
  return detailSrc.slice(start, end)
}

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

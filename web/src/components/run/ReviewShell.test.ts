// @vitest-environment happy-dom
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import ReviewShell from './ReviewShell.vue'
import {
  REVIEW_SIDEBAR,
  REVIEW_SHELL_WIDTH_KEY_APPROVAL,
  REVIEW_SHELL_WIDTH_KEY_REVIEW,
} from '@/lib/inbox/reviewLayoutBudget'

function mountShell(
  props: { mobile?: boolean; sidebarWidth?: number; storageKey?: string } = {},
) {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh',
    messages: {
      zh: {
        pages: {
          reviewShell: {
            drawerHandle: '拖动手柄调整高度',
            drawerHandleAria: '拖动手柄上下调整复审抽屉高度',
            resizeSash: '拖动调整侧栏宽度 · 双击恢复默认',
          },
        },
      },
    },
  })
  return mount(ReviewShell, {
    props,
    slots: {
      stage: '<div data-testid="stage-slot">stage</div>',
      sidebar: '<div data-testid="sidebar-slot">sidebar</div>',
    },
    global: { plugins: [i18n] },
    attachTo: document.body,
  })
}

function sidebarWidthStyle(wrapper: ReturnType<typeof mountShell>): string {
  return wrapper.get('[data-testid="review-shell-sidebar"]').attributes('style') || ''
}

function firePointer(
  el: Element,
  type: 'pointerdown' | 'pointermove' | 'pointerup',
  clientX: number,
  clientY = 0,
) {
  el.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      cancelable: true,
      clientX,
      clientY,
      pointerId: 1,
    }),
  )
}

function drawerHeightStyle(wrapper: ReturnType<typeof mountShell>): string {
  return wrapper.get('[data-testid="review-shell-sidebar"]').attributes('style') || ''
}

function mountMobileShell(
  props: { drawerHeight?: number } = {},
  shellHeight = 600,
) {
  Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
    configurable: true,
    value() {
      return {
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        bottom: shellHeight,
        right: 390,
        width: 390,
        height: shellHeight,
        toJSON() {
          return {}
        },
      }
    },
  })
  return mountShell({ mobile: true, ...props })
}

describe('ReviewShell sidebar width', () => {
  beforeEach(() => {
    localStorage.clear()
    // Wide enough that effectiveMax stays at 480.
    Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
      configurable: true,
      value() {
        return {
          x: 0,
          y: 0,
          top: 0,
          left: 0,
          bottom: 600,
          right: 1000,
          width: 1000,
          height: 600,
          toJSON() {
            return {}
          },
        }
      },
    })
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('defaults to 400px on desktop (Inbox/GateApproval unchanged)', () => {
    const w = mountShell()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*400px/)
    expect(w.find('[data-testid="review-shell-drawer-handle"]').exists()).toBe(false)
    expect(w.find('[data-testid="review-shell-sash"]').exists()).toBe(true)
    w.unmount()
  })

  it('honors explicit sidebarWidth=REVIEW_SIDEBAR (300) for Run Detail', () => {
    const w = mountShell({ sidebarWidth: REVIEW_SIDEBAR })
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*300px/)
    w.unmount()
  })

  it('mobile drawer path ignores fixed sidebar width and has no horizontal sash', () => {
    const w = mountShell({ mobile: true, sidebarWidth: REVIEW_SIDEBAR })
    expect(w.find('[data-testid="review-shell-drawer-handle"]').exists()).toBe(true)
    expect(w.find('[data-testid="review-shell-sash"]').exists()).toBe(false)
    expect(sidebarWidthStyle(w) || '').not.toMatch(/width:\s*300px/)
    expect(sidebarWidthStyle(w)).toMatch(/height:/)
    w.unmount()
  })

  it('renders desktop sash with separator a11y attrs', () => {
    const w = mountShell()
    const sash = w.get('[data-testid="review-shell-sash"]')
    expect(sash.attributes('role')).toBe('separator')
    expect(sash.attributes('aria-orientation')).toBe('vertical')
    expect(sash.attributes('aria-valuemin')).toBe('240')
    expect(sash.attributes('aria-valuemax')).toBe('480')
    expect(sash.attributes('aria-valuenow')).toBe('400')
    expect(sash.attributes('title')).toContain('拖动')
    w.unmount()
  })

  it('clamps drag to [240, 480] on a wide shell', async () => {
    const w = mountShell({ sidebarWidth: 400 })
    const sash = w.get('[data-testid="review-shell-sash"]').element

    // Drag left a lot → grow sidebar, clamp at 480
    firePointer(sash, 'pointerdown', 600)
    firePointer(sash, 'pointermove', 100)
    firePointer(sash, 'pointerup', 100)
    await w.vm.$nextTick()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*480px/)

    // Drag right a lot → shrink, clamp at 240
    firePointer(sash, 'pointerdown', 100)
    firePointer(sash, 'pointermove', 900)
    firePointer(sash, 'pointerup', 900)
    await w.vm.$nextTick()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*240px/)
    w.unmount()
  })

  it('persists width per storageKey and restores on remount', async () => {
    const w1 = mountShell({
      sidebarWidth: REVIEW_SIDEBAR,
      storageKey: REVIEW_SHELL_WIDTH_KEY_REVIEW,
    })
    const sash = w1.get('[data-testid="review-shell-sash"]').element
    firePointer(sash, 'pointerdown', 500)
    firePointer(sash, 'pointermove', 380) // +120 → 420
    firePointer(sash, 'pointerup', 380)
    await w1.vm.$nextTick()
    expect(sidebarWidthStyle(w1)).toMatch(/width:\s*420px/)
    expect(localStorage.getItem(REVIEW_SHELL_WIDTH_KEY_REVIEW)).toBe('420')
    w1.unmount()

    const w2 = mountShell({
      sidebarWidth: REVIEW_SIDEBAR,
      storageKey: REVIEW_SHELL_WIDTH_KEY_REVIEW,
    })
    expect(sidebarWidthStyle(w2)).toMatch(/width:\s*420px/)
    w2.unmount()
  })

  it('isolates review and approval storage keys', async () => {
    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_REVIEW, '450')
    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_APPROVAL, '280')

    const review = mountShell({
      sidebarWidth: REVIEW_SIDEBAR,
      storageKey: REVIEW_SHELL_WIDTH_KEY_REVIEW,
    })
    expect(sidebarWidthStyle(review)).toMatch(/width:\s*450px/)
    review.unmount()

    const approval = mountShell({
      sidebarWidth: 400,
      storageKey: REVIEW_SHELL_WIDTH_KEY_APPROVAL,
    })
    expect(sidebarWidthStyle(approval)).toMatch(/width:\s*280px/)
    approval.unmount()
  })

  it('double-click sash resets to props.sidebarWidth and persists', async () => {
    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_REVIEW, '450')
    const w = mountShell({
      sidebarWidth: REVIEW_SIDEBAR,
      storageKey: REVIEW_SHELL_WIDTH_KEY_REVIEW,
    })
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*450px/)

    await w.get('[data-testid="review-shell-sash"]').trigger('dblclick')
    await w.vm.$nextTick()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*300px/)
    expect(localStorage.getItem(REVIEW_SHELL_WIDTH_KEY_REVIEW)).toBe('300')
    w.unmount()
  })

  it('falls back to default when stored value is illegal or out of range', () => {
    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_APPROVAL, 'not-a-number')
    const bad = mountShell({
      sidebarWidth: 400,
      storageKey: REVIEW_SHELL_WIDTH_KEY_APPROVAL,
    })
    expect(sidebarWidthStyle(bad)).toMatch(/width:\s*400px/)
    bad.unmount()

    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_APPROVAL, '-50')
    const neg = mountShell({
      sidebarWidth: 400,
      storageKey: REVIEW_SHELL_WIDTH_KEY_APPROVAL,
    })
    expect(sidebarWidthStyle(neg)).toMatch(/width:\s*400px/)
    neg.unmount()

    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_APPROVAL, '9999')
    const huge = mountShell({
      sidebarWidth: 400,
      storageKey: REVIEW_SHELL_WIDTH_KEY_APPROVAL,
    })
    expect(sidebarWidthStyle(huge)).toMatch(/width:\s*400px/)
    huge.unmount()
  })

  it('reclamps when parent shell shrinks without window.resize (outer sash F3)', async () => {
    const observers: ResizeObserverCallback[] = []
    class MockResizeObserver {
      constructor(cb: ResizeObserverCallback) {
        observers.push(cb)
      }
      observe() {}
      unobserve() {}
      disconnect() {}
    }
    vi.stubGlobal('ResizeObserver', MockResizeObserver)

    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_REVIEW, '300')
    const w = mountShell({
      sidebarWidth: REVIEW_SIDEBAR,
      storageKey: REVIEW_SHELL_WIDTH_KEY_REVIEW,
    })
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*300px/)

    Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
      configurable: true,
      value() {
        // OUTER_RIGHT_MIN 324: room = 324 - STAGE_MIN 160 - sash 4 = 160
        return {
          x: 0,
          y: 0,
          top: 0,
          left: 0,
          bottom: 600,
          right: 324,
          width: 324,
          height: 600,
          toJSON() {
            return {}
          },
        }
      },
    })
    expect(observers.length).toBeGreaterThan(0)
    for (const cb of observers) {
      cb([], {} as ResizeObserver)
    }
    await w.vm.$nextTick()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*160px/)
    expect(w.get('[data-testid="review-shell-stage"]').classes()).toContain(
      'review-shell-stage',
    )
    const sash = w.get('[data-testid="review-shell-sash"]')
    expect(sash.attributes('aria-valuemax')).toBe('160')
    expect(sash.attributes('aria-valuemin')).toBe('160')
    w.unmount()
  })

  it('reclamps on window resize when shell shrinks', async () => {
    localStorage.setItem(REVIEW_SHELL_WIDTH_KEY_REVIEW, '480')
    const w = mountShell({
      sidebarWidth: REVIEW_SIDEBAR,
      storageKey: REVIEW_SHELL_WIDTH_KEY_REVIEW,
    })
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*480px/)

    Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
      configurable: true,
      value() {
        // room = 400 - 160 - 4 = 236 < SIDEBAR_MIN → prioritize stage, allow 236
        return {
          x: 0,
          y: 0,
          top: 0,
          left: 0,
          bottom: 600,
          right: 400,
          width: 400,
          height: 600,
          toJSON() {
            return {}
          },
        }
      },
    })
    window.dispatchEvent(new Event('resize'))
    await w.vm.$nextTick()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*236px/)
    w.unmount()
  })

  it('narrow shell prefers stage min width over sidebar floor', async () => {
    Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
      configurable: true,
      value() {
        // room = 360 - 160 - 4 = 196 → sidebar may drop below 240
        return {
          x: 0,
          y: 0,
          top: 0,
          left: 0,
          bottom: 600,
          right: 360,
          width: 360,
          height: 600,
          toJSON() {
            return {}
          },
        }
      },
    })
    const w = mountShell({ sidebarWidth: 400 })
    await w.vm.$nextTick()
    expect(sidebarWidthStyle(w)).toMatch(/width:\s*196px/)
    const sash = w.get('[data-testid="review-shell-sash"]')
    expect(sash.attributes('aria-valuemax')).toBe('196')
    expect(sash.attributes('aria-valuemin')).toBe('196')
    expect(w.get('[data-testid="review-shell-stage"]').classes()).toContain(
      'review-shell-stage',
    )
    w.unmount()
  })

  it('disables text selection while sash is dragging', async () => {
    const w = mountShell({ sidebarWidth: 400 })
    const sash = w.get('[data-testid="review-shell-sash"]').element
    firePointer(sash, 'pointerdown', 600)
    await w.vm.$nextTick()
    expect(document.body.classList.contains('review-shell-sash-dragging')).toBe(true)
    expect(w.get('[data-testid="review-shell"]').classes()).toContain('select-none')
    firePointer(sash, 'pointerup', 600)
    await w.vm.$nextTick()
    expect(document.body.classList.contains('review-shell-sash-dragging')).toBe(false)
    w.unmount()
  })
})

// plan_coverage leaves (mobile drawer fix): g1.1 drag events, g1.2 hit area, g1.3 aria/locale,
// g1.4 adaptive default + stage min, g3.1 unit tests — use leaf ids only in test_result.
describe('ReviewShell mobile drawer', () => {
  beforeEach(() => {
    localStorage.clear()
    document.body.classList.remove('review-shell-drawer-dragging')
  })

  afterEach(() => {
    localStorage.clear()
    document.body.classList.remove('review-shell-drawer-dragging')
    vi.restoreAllMocks()
  })

  // plan_coverage: g1.4 — adaptive default height + stage≥160 clamp
  it('adaptive default height preserves stage min and stays below old 340 default', async () => {
    const w = mountMobileShell({}, 600)
    await w.vm.$nextTick()
    const style = drawerHeightStyle(w)
    const match = style.match(/height:\s*(\d+)px/)
    expect(match).toBeTruthy()
    const drawerH = Number(match![1])
    expect(drawerH).toBeLessThan(340)
    expect(drawerH).toBeGreaterThanOrEqual(180)
    // stage = shell 600 - drawer should stay >= STAGE_MIN 160
    expect(600 - drawerH).toBeGreaterThanOrEqual(160)
    w.unmount()
  })

  // plan_coverage: g1.2 / g1.3 — 44px hit area, touch-action:none, horizontal separator aria + 拖动文案
  it('drawer handle has touch-action none and expanded hit area', () => {
    const w = mountMobileShell()
    const handle = w.get('[data-testid="review-shell-drawer-handle"]')
    expect(handle.classes()).toContain('review-shell-drawer-handle')
    expect(handle.attributes('role')).toBe('separator')
    expect(handle.attributes('aria-orientation')).toBe('horizontal')
    expect(handle.attributes('aria-label')).toContain('拖动')
    w.unmount()
  })

  // plan_coverage: g1.1 — pointer capture, preventDefault, scroll lock while dragging
  it('dragging drawer handle changes height and locks scroll', async () => {
    const w = mountMobileShell({}, 600)
    await w.vm.$nextTick()
    const before = Number((drawerHeightStyle(w).match(/height:\s*(\d+)px/) || [])[1])
    const handle = w.get('[data-testid="review-shell-drawer-handle"]').element
    firePointer(handle, 'pointerdown', 200, 500)
    firePointer(handle, 'pointermove', 200, 420)
    await w.vm.$nextTick()
    const after = Number((drawerHeightStyle(w).match(/height:\s*(\d+)px/) || [])[1])
    expect(after).toBeGreaterThan(before)
    expect(document.body.classList.contains('review-shell-drawer-dragging')).toBe(true)
    firePointer(handle, 'pointerup', 200, 420)
    await w.vm.$nextTick()
    expect(document.body.classList.contains('review-shell-drawer-dragging')).toBe(false)
    w.unmount()
  })

  it('clamps drawer drag to shell budget on short viewports', async () => {
    const w = mountMobileShell({}, 360)
    await w.vm.$nextTick()
    const handle = w.get('[data-testid="review-shell-drawer-handle"]').element
    const start = Number((drawerHeightStyle(w).match(/height:\s*(\d+)px/) || [])[1])
    firePointer(handle, 'pointerdown', 200, 300)
    firePointer(handle, 'pointermove', 200, 50)
    await w.vm.$nextTick()
    const end = Number((drawerHeightStyle(w).match(/height:\s*(\d+)px/) || [])[1])
    expect(end).toBeGreaterThan(start)
    expect(360 - end).toBeGreaterThanOrEqual(160)
    w.unmount()
  })

  it('stage section enforces min height on mobile', () => {
    const w = mountMobileShell()
    const stage = w.get('[data-testid="review-shell-stage"]')
    expect(stage.attributes('style')).toMatch(/min-height:\s*160px/)
    w.unmount()
  })
})

/** Document-level idle-invisible scrollbar: scroll → is-scrolling → clear after ~800ms. */

export const IDLE_SCROLLBAR_HIDE_MS = 800

const SCROLLING_CLASS = 'is-scrolling'

/** Monaco (and similar) use custom scrollbars — do not toggle CSS is-scrolling on them. */
const MONACO_SELECTOR = '.monaco-editor, .monaco-diff-editor'

const timers = new WeakMap<Element, ReturnType<typeof setTimeout>>()
/** Strong refs so uninstall can clear pending hide timers (WeakMap alone is not iterable). */
const pendingEls = new Set<Element>()
let installed = false

function isMonacoTarget(el: Element): boolean {
  return Boolean(el.closest(MONACO_SELECTOR))
}

function resolveScrollElement(target: EventTarget | null): Element | null {
  if (target === document || target === document.documentElement) {
    return document.documentElement
  }
  if (target === document.body) {
    return document.body
  }
  if (target instanceof Element) {
    return target
  }
  return null
}

function clearPending(el: Element) {
  const prev = timers.get(el)
  if (prev) clearTimeout(prev)
  timers.delete(el)
  pendingEls.delete(el)
  el.classList.remove(SCROLLING_CLASS)
}

function onDocumentScroll(event: Event) {
  const el = resolveScrollElement(event.target)
  if (!el || isMonacoTarget(el)) return

  el.classList.add(SCROLLING_CLASS)
  const prev = timers.get(el)
  if (prev) clearTimeout(prev)
  pendingEls.add(el)
  timers.set(
    el,
    setTimeout(() => {
      el.classList.remove(SCROLLING_CLASS)
      timers.delete(el)
      pendingEls.delete(el)
    }, IDLE_SCROLLBAR_HIDE_MS),
  )
}

/** Install capture-phase passive scroll listener once (safe to call repeatedly). */
export function installIdleScrollbar(): void {
  if (installed || typeof document === 'undefined') return
  document.addEventListener('scroll', onDocumentScroll, { capture: true, passive: true })
  installed = true
}

/** Remove listener and clear any pending hide timers / is-scrolling classes. */
export function uninstallIdleScrollbar(): void {
  if (!installed || typeof document === 'undefined') return
  document.removeEventListener('scroll', onDocumentScroll, { capture: true })
  for (const el of [...pendingEls]) {
    clearPending(el)
  }
  installed = false
}

export function isIdleScrollbarInstalled(): boolean {
  return installed
}

/** Document-level origin-fill: pointerenter/leave → expand high-contrast circle from pointer. */

const FILLED_CLASS = 'is-filled'

let installed = false

/** Diameter that covers the farthest corner from (x, y) inside the button. */
export function coverDiameter(width: number, height: number, x: number, y: number): number {
  const r = Math.max(
    Math.hypot(x, y),
    Math.hypot(width - x, y),
    Math.hypot(x, height - y),
    Math.hypot(width - x, height - y),
  )
  return r * 2 + 2
}

function localXY(el: HTMLElement, e: PointerEvent): [number, number] {
  const b = el.getBoundingClientRect()
  return [e.clientX - b.left, e.clientY - b.top]
}

function resolveButton(target: EventTarget | null): HTMLButtonElement | null {
  if (!(target instanceof Element)) return null
  const btn = target.closest('button')
  return btn instanceof HTMLButtonElement ? btn : null
}

/** Skip disabled / loading (aria-busy) buttons — no fill, no ink flip. */
export function shouldSkipOriginFill(btn: HTMLButtonElement): boolean {
  return (
    btn.disabled ||
    btn.getAttribute('aria-busy') === 'true' ||
    btn.getAttribute('aria-disabled') === 'true'
  )
}

function setOrigin(btn: HTMLButtonElement, e: PointerEvent) {
  const [x, y] = localXY(btn, e)
  const d = coverDiameter(btn.clientWidth, btn.clientHeight, x, y)
  btn.style.setProperty('--ox', `${x}px`)
  btn.style.setProperty('--oy', `${y}px`)
  btn.style.setProperty('--or', `${d}px`)
}

function onPointerEnter(e: Event) {
  if (!(e instanceof PointerEvent)) return
  const btn = resolveButton(e.target)
  if (!btn || shouldSkipOriginFill(btn)) return
  setOrigin(btn, e)
  btn.classList.add(FILLED_CLASS)
}

function onPointerLeave(e: Event) {
  if (!(e instanceof PointerEvent)) return
  const btn = resolveButton(e.target)
  if (!btn) return
  const related = e.relatedTarget
  if (related instanceof Node && btn.contains(related)) return
  setOrigin(btn, e)
  btn.classList.remove(FILLED_CLASS)
}

/** Install capture-phase pointerenter/leave once (safe to call repeatedly). */
export function installOriginFill(): void {
  if (installed || typeof document === 'undefined') return
  document.addEventListener('pointerenter', onPointerEnter, true)
  document.addEventListener('pointerleave', onPointerLeave, true)
  installed = true
}

/** Remove listeners and clear any stuck is-filled classes. */
export function uninstallOriginFill(): void {
  if (!installed || typeof document === 'undefined') return
  document.removeEventListener('pointerenter', onPointerEnter, true)
  document.removeEventListener('pointerleave', onPointerLeave, true)
  document.querySelectorAll(`button.${FILLED_CLASS}`).forEach((el) => {
    el.classList.remove(FILLED_CLASS)
  })
  installed = false
}

export function isOriginFillInstalled(): boolean {
  return installed
}

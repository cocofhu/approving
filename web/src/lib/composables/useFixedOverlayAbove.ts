import { nextTick, onBeforeUnmount, type Ref } from 'vue'

const VIEWPORT_MARGIN = 8
const DEFAULT_GAP = 6

export type FixedOverlayAboveStyle = {
  position: 'fixed'
  top: string
  left: string
  width?: string
}

/**
 * Place a fixed overlay directly above an anchor (sidebar footer chrome).
 * Clamps into the viewport without flipping below the anchor.
 */
export async function placeFixedOverlayAbove(
  anchor: HTMLElement | null,
  overlay: HTMLElement | null,
  options?: { gap?: number; align?: 'left' | 'center'; width?: number },
): Promise<FixedOverlayAboveStyle | null> {
  if (!anchor || !overlay) return null
  await nextTick()
  const gap = options?.gap ?? DEFAULT_GAP
  const rect = anchor.getBoundingClientRect()
  const overlayRect = overlay.getBoundingClientRect()
  const width = options?.width ?? overlayRect.width

  let left =
    options?.align === 'left'
      ? rect.left
      : rect.left + rect.width / 2 - width / 2
  left = Math.min(window.innerWidth - width - VIEWPORT_MARGIN, Math.max(VIEWPORT_MARGIN, left))

  let top = rect.top - overlayRect.height - gap
  top = Math.max(VIEWPORT_MARGIN, top)

  return {
    position: 'fixed',
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
    ...(options?.width ? { width: `${options.width}px` } : {}),
  }
}

export function useFixedOverlayAboveListeners(
  open: Ref<boolean>,
  reposition: () => void | Promise<void>,
) {
  const onViewportChange = () => {
    if (open.value) void reposition()
  }

  const start = () => {
    window.addEventListener('resize', onViewportChange)
    window.addEventListener('scroll', onViewportChange, true)
  }

  const stop = () => {
    window.removeEventListener('resize', onViewportChange)
    window.removeEventListener('scroll', onViewportChange, true)
  }

  onBeforeUnmount(stop)

  return { start, stop, onViewportChange }
}

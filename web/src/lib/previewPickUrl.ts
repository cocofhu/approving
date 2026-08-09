/**
 * Path (+ search + hash) from a full href for compact pick/chip labels.
 * Returns "" when href is empty or not a valid absolute URL.
 */
export function previewPickPath(href: string | undefined | null): string {
  const raw = (href || '').trim()
  if (!raw) return ''
  try {
    const u = new URL(raw)
    return `${u.pathname}${u.search}${u.hash}` || '/'
  } catch {
    // Relative or opaque — treat as already-path-like.
    if (raw.startsWith('/')) return raw
    return ''
  }
}

/** Chip / annotation label: `path · selector`, or selector when path missing. */
export function previewPickLabel(
  href: string | undefined | null,
  selector: string,
  tagName?: string,
): string {
  const sel = (selector || '').trim() || (tagName || '').trim()
  const path = previewPickPath(href)
  if (path && sel) return `${path} · ${sel}`
  return sel || path
}

export type AppPreviewPickPayload = {
  selector: string
  tagName: string
  outerHTML: string
  url?: string
}

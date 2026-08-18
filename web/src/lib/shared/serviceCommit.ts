/** 7-char lowercase hex SHA used as the overview-page service version badge. */
const SHORT_LEN = 7
const HEX = /^[0-9a-f]+$/

/**
 * Normalize a service-program revision to a 7-char short SHA.
 * Empty, whitespace, non-hex, or shorter than 7 hex digits → "" (caller hides the badge).
 * Longer values are truncated; no unknown/N/A/— placeholders.
 */
export function normalizeShortSha(raw: unknown): string {
  if (typeof raw !== 'string') return ''
  const s = raw.trim().toLowerCase()
  if (!s || !HEX.test(s) || s.length < SHORT_LEN) return ''
  return s.slice(0, SHORT_LEN)
}

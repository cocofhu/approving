import type { ReactAnnotation } from '@/lib/types'

/** Soft cap for a single paragraph quote sent to the agent / shown in chips. */
export const REVIEW_QUOTE_LIMIT = 500

export type AddAnnotationResult = 'added' | 'duplicate' | 'ignored'

/** Normalize + soft-truncate quote text (~500 chars). */
export function truncateQuote(
  text: string,
  limit = REVIEW_QUOTE_LIMIT,
): { quote: string; truncated: boolean } {
  const normalized = String(text || '')
    .replace(/\s+/g, ' ')
    .trim()
  if (!normalized) return { quote: '', truncated: false }
  if (normalized.length <= limit) return { quote: normalized, truncated: false }
  return { quote: normalized.slice(0, limit), truncated: true }
}

/** True when the annotation carries a paragraph quote (vs whole-field / selector). */
export function isQuoteAnnotation(ann: ReactAnnotation): boolean {
  return !!(ann.quote && ann.quote.trim())
}

/**
 * Dedup key:
 * - quote annotations → quote + path (path may be empty for unbound)
 * - whole-field / selector → path/selector only
 */
export function annotationDedupeKey(ann: ReactAnnotation): string {
  if (isQuoteAnnotation(ann)) {
    return `quote|${ann.jsonPath || ''}|${(ann.quote || '').trim()}`
  }
  return `field|${ann.jsonPath || ann.selector || ''}`
}

/** Apply quote soft-cap; leave field/selector annotations unchanged. */
export function prepareAnnotation(ann: ReactAnnotation): ReactAnnotation {
  if (!isQuoteAnnotation(ann)) return { ...ann }
  const { quote, truncated } = truncateQuote(ann.quote || '')
  return {
    ...ann,
    quote,
    truncated: truncated || !!ann.truncated,
  }
}

/**
 * Append one annotation with quote-aware dedup.
 * Returns 'duplicate' when the same key already exists, 'ignored' for empty keys.
 */
export function pushAnnotationUnique(
  list: ReactAnnotation[],
  raw: ReactAnnotation,
): AddAnnotationResult {
  const ann = prepareAnnotation(raw)
  if (isQuoteAnnotation(ann)) {
    if (!ann.quote) return 'ignored'
    const key = annotationDedupeKey(ann)
    if (list.some((a) => annotationDedupeKey(a) === key)) return 'duplicate'
    list.push(ann)
    return 'added'
  }
  const ref = ann.jsonPath || ann.selector || ''
  // Unbound empty field annotation is useless; still allow quote-less with neither ref
  // only when something identifiable exists (label alone is not a ref for dedup).
  if (!ref) {
    // Preserve prior behaviour: no-ref field chips are not pushed via AnnotateBtn
    // (AnnotateBtn always sets jsonPath). Keep push for empty-ref as no-op.
    return 'ignored'
  }
  const key = annotationDedupeKey(ann)
  if (list.some((a) => annotationDedupeKey(a) === key)) return 'duplicate'
  list.push(ann)
  return 'added'
}

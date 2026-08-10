import { highlightJson } from './highlightJson'

/**
 * Pretty-print audit payload JSON for v-html display.
 * Always HTML-escapes first (via highlightJson) so stored payloads cannot XSS.
 */
export function prettyAuditPayload(payload: Record<string, unknown> | undefined): string {
  const json = JSON.stringify(payload ?? {}, null, 2)
  return highlightJson(json).replace(
    /(&quot;)(\*{4})(&quot;)/g,
    '$1<span class="audit-mask">$2</span>$3',
  )
}

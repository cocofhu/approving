import { describe, expect, it } from 'vitest'
import { prettyAuditPayload } from './auditPayload'

describe('prettyAuditPayload', () => {
  it('HTML-escapes angle brackets to prevent stored XSS', () => {
    const html = prettyAuditPayload({
      form: { note: '<img onerror=alert(1)>' },
    })
    expect(html).not.toContain('<img')
    expect(html).toContain('&lt;img onerror=alert(1)&gt;')
  })

  it('HTML-escapes ampersands and quotes in string values', () => {
    const html = prettyAuditPayload({ msg: 'a & b "c"' })
    expect(html).toContain('&amp;')
    expect(html).toContain('&quot;')
    // JSON encodes value quotes as \"; after escapeHtml they become \&quot; entities.
    // Do not use /[^;]">/ — it false-positives on legitimate class="tok-*"> spans.
    expect(html).toContain('a &amp; b \\&quot;c\\&quot;')
    const contentOnly = html.replace(/<span class="[^"]*">/g, '').replace(/<\/span>/g, '')
    expect(contentOnly).not.toContain('"')
  })

  it('highlights **** mask values', () => {
    const html = prettyAuditPayload({ token: '****' })
    expect(html).toContain('audit-mask')
    expect(html).toContain('****')
  })
})

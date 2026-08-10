import { marked } from 'marked'
import DOMPurify from 'dompurify'

marked.setOptions({ breaks: true, gfm: true })

/** Max distinct source strings retained (LRU via Map insertion order). */
const CACHE_MAX = 64

const htmlCache = new Map<string, string>()

/** Test/observability: how many times marked+DOMPurify actually ran. */
let parseCount = 0

export function getMarkdownParseCount(): number {
  return parseCount
}

export function resetMarkdownParseCount(): void {
  parseCount = 0
}

export function clearMarkdownCache(): void {
  htmlCache.clear()
}

export function markdownCacheSize(): number {
  return htmlCache.size
}

/**
 * Render markdown → sanitized HTML with an LRU cache keyed by source text.
 * Identical inputs reuse the previous HTML and skip marked+DOMPurify.
 */
export function renderMarkdown(src: string): string {
  const key = src ?? ''
  const hit = htmlCache.get(key)
  if (hit !== undefined) {
    // Refresh LRU order
    htmlCache.delete(key)
    htmlCache.set(key, hit)
    return hit
  }
  parseCount += 1
  const raw = marked.parse(key, { async: false }) as string
  const html = DOMPurify.sanitize(raw)
  htmlCache.set(key, html)
  if (htmlCache.size > CACHE_MAX) {
    const oldest = htmlCache.keys().next().value
    if (oldest !== undefined) htmlCache.delete(oldest)
  }
  return html
}

/**
 * Coalesce rapid stream text updates into at most one markdown render per
 * animation frame so token deltas cannot flood marked+DOMPurify.
 */

export type StreamMarkdownPreviewOptions = {
  render: (src: string) => string
  /** Inject for tests; defaults to requestAnimationFrame. */
  schedule?: (cb: () => void) => number
  cancel?: (id: number) => void
}

export type StreamMarkdownPreview = {
  /** Absolute replace (resume snapshot / clear). */
  setText: (text: string) => void
  /** Append a delta and schedule a coalesced render. */
  append: (delta: string) => void
  /** Current raw stream text. */
  getText: () => string
  /** Last rendered HTML (may lag text by up to one frame). */
  getHtml: () => string
  /** Force flush (tests / end-of-stream). */
  flush: () => void
  /** Cancel pending frame and clear. */
  reset: () => void
  /** Subscribe to HTML updates; returns unsubscribe. */
  subscribe: (listener: (html: string) => void) => () => void
}

export function createStreamMarkdownPreview(
  opts: StreamMarkdownPreviewOptions,
): StreamMarkdownPreview {
  const schedule = opts.schedule ?? ((cb) => requestAnimationFrame(cb))
  const cancel = opts.cancel ?? ((id) => cancelAnimationFrame(id))
  let text = ''
  let html = ''
  let pending = false
  let handle = 0
  const listeners = new Set<(html: string) => void>()

  function notify() {
    for (const l of listeners) l(html)
  }

  function flush() {
    if (pending) {
      cancel(handle)
      pending = false
      handle = 0
    }
    html = text ? opts.render(text) : ''
    notify()
  }

  function scheduleFlush() {
    if (pending) return
    pending = true
    handle = schedule(() => flush())
  }

  return {
    setText(next: string) {
      text = next
      scheduleFlush()
    },
    append(delta: string) {
      if (!delta) return
      text += delta
      scheduleFlush()
    },
    getText: () => text,
    getHtml: () => html,
    flush,
    reset() {
      text = ''
      html = ''
      if (pending) {
        cancel(handle)
        pending = false
        handle = 0
      }
      notify()
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
  }
}

/** Idle-invisible ghost scrollbar for demo iframe srcdoc; mirrors global.css (rgb literals). */

export const DEMO_GHOST_SCROLLBAR_STYLE = `<style>
* {
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}
*:hover,
*.is-scrolling {
  scrollbar-color: rgb(38, 38, 43) transparent;
}
::-webkit-scrollbar {
  width: 4px;
  height: 4px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: transparent;
  border-radius: 0;
  transition: background 0.15s ease;
}
*:hover::-webkit-scrollbar-thumb,
*.is-scrolling::-webkit-scrollbar-thumb {
  background: rgb(38, 38, 43);
}
*:hover::-webkit-scrollbar-thumb:hover,
*.is-scrolling::-webkit-scrollbar-thumb:hover {
  background: rgb(54, 54, 62);
}
</style>`

/** Lightweight scroll → is-scrolling → ~800ms hide (align page.html bindGhost). */
export const DEMO_GHOST_SCROLLBAR_SCRIPT = `<script>
(function () {
  var timers = new WeakMap();
  document.addEventListener('scroll', function (e) {
    var el = e.target;
    if (el === document || el === document.documentElement) el = document.documentElement;
    else if (el === document.body) el = document.body;
    if (!el || !el.classList) return;
    el.classList.add('is-scrolling');
    var prev = timers.get(el);
    if (prev) clearTimeout(prev);
    timers.set(el, setTimeout(function () {
      el.classList.remove('is-scrolling');
      timers.delete(el);
    }, 800));
  }, { capture: true, passive: true });
})();
<\/script>`

function injectBeforeCloseTag(html: string, closeTag: string, snippet: string): string | null {
  const idx = html.toLowerCase().indexOf(closeTag)
  if (idx === -1) return null
  return html.slice(0, idx) + snippet + html.slice(idx)
}

export function injectDemoScrollbarStyles(html: string): string {
  if (!html) return html

  let result = html

  const headCloseIdx = result.toLowerCase().indexOf('</head>')
  if (headCloseIdx !== -1) {
    result = result.slice(0, headCloseIdx) + DEMO_GHOST_SCROLLBAR_STYLE + result.slice(headCloseIdx)
  } else {
    const headOpenMatch = result.match(/<head\b[^>]*>/i)
    if (headOpenMatch?.index !== undefined) {
      const insertIdx = headOpenMatch.index + headOpenMatch[0].length
      result = result.slice(0, insertIdx) + DEMO_GHOST_SCROLLBAR_STYLE + result.slice(insertIdx)
    } else {
      const htmlOpenMatch = result.match(/<html\b[^>]*>/i)
      if (htmlOpenMatch?.index !== undefined) {
        const insertIdx = htmlOpenMatch.index + htmlOpenMatch[0].length
        result =
          result.slice(0, insertIdx) +
          `<head>${DEMO_GHOST_SCROLLBAR_STYLE}</head>` +
          result.slice(insertIdx)
      } else {
        result = DEMO_GHOST_SCROLLBAR_STYLE + result
      }
    }
  }

  const withBodyScript = injectBeforeCloseTag(result, '</body>', DEMO_GHOST_SCROLLBAR_SCRIPT)
  if (withBodyScript) return withBodyScript

  return result + DEMO_GHOST_SCROLLBAR_SCRIPT
}

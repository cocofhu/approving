function makeEl(tag) {
  const attrs = {}
  return {
    tagName: String(tag || 'div').toUpperCase(),
    style: {},
    childNodes: [],
    children: [],
    attributes: attrs,
    setAttribute(k, v) { attrs[k] = v },
    getAttribute(k) { return attrs[k] },
    appendChild(c) { this.childNodes.push(c); return c },
    removeChild() {},
    addEventListener() {},
    removeEventListener() {},
    textContent: '',
    innerHTML: '',
    classList: { add() {}, remove() {}, contains() { return false } },
  }
}
const document = {
  createElement: makeEl,
  createElementNS: (_ns, tag) => makeEl(tag),
  createTextNode: (t) => ({ textContent: t }),
  body: makeEl('body'),
  head: makeEl('head'),
  documentElement: makeEl('html'),
  querySelector: () => null,
  querySelectorAll: () => [],
  getElementById: () => null,
}
const window = {
  document,
  DOMParser: class {
    parseFromString() { return document }
  },
  addEventListener() {},
  getComputedStyle: () => ({}),
}
Object.defineProperty(globalThis, 'window', { value: window, configurable: true })
Object.defineProperty(globalThis, 'document', { value: document, configurable: true })
Object.defineProperty(globalThis, 'DOMParser', { value: window.DOMParser, configurable: true })

import DOMPurify from 'dompurify'
globalThis.DOMPurify = DOMPurify
window.DOMPurify = DOMPurify

async function main() {
  const mermaid = (await import('mermaid')).default
  mermaid.initialize({ startOnLoad: false, suppressErrorRendering: true, securityLevel: 'strict' })

  let source
  if (process.argv[2] != null && process.argv[2] !== '-') {
    source = process.argv[2]
  } else {
    const chunks = []
    for await (const c of process.stdin) chunks.push(c)
    source = Buffer.concat(chunks).toString('utf8')
  }
  source = source.trim()
  if (!source) {
    process.stderr.write('empty source\n')
    process.exit(2)
  }
  try {
    await mermaid.parse(source)
    process.stdout.write('ok\n')
    process.exit(0)
  } catch (e) {
    const msg = e && e.message ? e.message : String(e)
    process.stderr.write(msg.replace(/\n/g, ' ') + '\n')
    process.exit(1)
  }
}

main()

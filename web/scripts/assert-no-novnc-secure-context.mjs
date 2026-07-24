#!/usr/bin/env node
/**
 * Fail if a production Vite build still embeds noVNC's Secure Context warning.
 * Catches HTTP (non-localhost) deployment regressions that localhost e2e miss.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

const SNIPPET = 'requires a secure context'
const dist = join(process.cwd(), 'dist')

function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) out.push(...walk(p))
    else if (/\.(js|mjs|cjs)$/.test(name)) out.push(p)
  }
  return out
}

let found = []
try {
  found = walk(dist).filter((f) => readFileSync(f, 'utf8').includes(SNIPPET))
} catch (e) {
  console.error(`assert-no-novnc-secure-context: cannot read ${dist}:`, e.message)
  process.exit(1)
}

if (found.length) {
  console.error('noVNC Secure Context warning still present in build output:')
  for (const f of found) console.error(' ', f)
  process.exit(1)
}
console.log('ok: no Secure Context warning in dist/')

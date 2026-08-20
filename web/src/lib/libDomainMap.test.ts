import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const map = JSON.parse(readFileSync(join(here, '../../LIB_DOMAIN_MAP.json'), 'utf8')) as {
  domains: Record<string, string[]>
  totals: Record<string, number>
}

function listModules(dir: string): string[] {
  return readdirSync(dir)
    .filter((name) => name.endsWith('.ts') && !name.endsWith('.test.ts') && !name.includes('.test.'))
    .sort()
}

describe('LIB_DOMAIN_MAP', () => {
  it('lists every non-test lib module and no extras', () => {
    const diskDomains = readdirSync(here, { withFileTypes: true })
      .filter((e) => e.isDirectory())
      .map((e) => e.name)
      .sort()
    expect(Object.keys(map.domains).sort()).toEqual(diskDomains)

    for (const domain of diskDomains) {
      const files = listModules(join(here, domain))
      expect(map.domains[domain], domain).toEqual(files)
      expect(map.totals[domain], domain).toBe(files.length)
    }
  })
})

// @vitest-environment node
/**
 * Radius zero-clearance gate (plan g4): no rounded-none leftovers in product
 * sources, plus hard contracts for home / switch / demo / analytics / login.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const srcRoot = join(here, '..')

const SCAN_EXTS = new Set(['.vue', '.ts', '.css'])

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === 'dist') continue
    const full = join(dir, name)
    const st = statSync(full)
    if (st.isDirectory()) {
      walk(full, out)
      continue
    }
    const lower = name.toLowerCase()
    // Skip *.test.ts / *.spec.ts; keep product .ts/.vue/.css
    if (/\.(test|spec)\.ts$/.test(lower)) continue
    const dot = lower.lastIndexOf('.')
    const ext = dot >= 0 ? lower.slice(dot) : ''
    if (!SCAN_EXTS.has(ext)) continue
    out.push(full)
  }
  return out
}

function read(relFromSrc: string): string {
  return readFileSync(join(srcRoot, relFromSrc), 'utf8')
}

describe('radius zero clearance', () => {
  it('has no rounded-none in web/src product .vue / non-test .ts / .css', () => {
    const hits: string[] = []
    for (const file of walk(srcRoot)) {
      const text = readFileSync(file, 'utf8')
      if (!text.includes('rounded-none')) continue
      // Line-level report for actionable failures
      text.split(/\r?\n/).forEach((line, i) => {
        if (line.includes('rounded-none')) {
          hits.push(`${relative(srcRoot, file)}:${i + 1}:${line.trim().slice(0, 160)}`)
        }
      })
    }
    expect(hits).toEqual([])
  })

  it('DashboardView home composer uses shell 16px and toolbar controls 8px', () => {
    const src = read('views/DashboardView.vue')
    expect(src).toMatch(/\.home-composer\s*\{[^}]*border-radius:\s*16px/s)
    expect(src).toMatch(/\.home-composer__plus\s*\{[^}]*border-radius:\s*8px/s)
    expect(src).toMatch(/\.home-composer__send\s*\{[^}]*border-radius:\s*8px/s)
    expect(src).not.toMatch(/home-composer__plus[^"]*rounded-none/)
    expect(src).not.toMatch(/thumb-class="rounded-none"/)
  })

  it('AppSwitch uses capsule rounded-full (not rounded-none)', () => {
    const src = read('components/ui/AppSwitch.vue')
    expect(src).toMatch(/role="switch"[^>]*class="[^"]*\brounded-full\b/)
    expect(src).not.toMatch(/rounded-none/)
  })

  it('ClarifyDemoFrame root uses card radius rounded-lg', () => {
    const src = read('components/run/ClarifyDemoFrame.vue')
    expect(src).toMatch(/class="[^"]*\brounded-lg\b[^"]*border/)
  })

  it('TokenAnalyticsView filter selects and clear use control rounded', () => {
    const src = read('views/TokenAnalyticsView.vue')
    expect(src).toMatch(
      /v-model="projectSel"\s+class="[^"]*\brounded\b[^"]*\bborder border-line\b/,
    )
    expect(src).toMatch(
      /v-model="modelSel"\s+class="[^"]*\brounded\b[^"]*\bborder border-line\b/,
    )
    expect(src).toMatch(
      /<button[^>]*class="[^"]*\brounded\b[^"]*"[^>]*@click="clearFilters"/,
    )
  })

  it('LoginView login card uses shell radius rounded-xl', () => {
    const src = read('views/LoginView.vue')
    expect(src).toMatch(/class="[^"]*\brounded-xl\b[^"]*border border-line bg-surface/)
  })

  it('HomePipelineSelect trigger 8px and panel 12px', () => {
    const src = read('components/dashboard/HomePipelineSelect.vue')
    expect(src).toMatch(/\.home-pipeline-select__trigger\s*\{[^}]*border-radius:\s*8px/s)
    expect(src).toMatch(/\.home-pipeline-select__panel\s*\{[^}]*border-radius:\s*12px/s)
    expect(src).toMatch(/\.home-pipeline-select__search\s*\{[^}]*border-radius:\s*8px/s)
  })
})

#!/usr/bin/env node
/**
 * Brand clear guard: fail CI when product protocol/storage identifiers
 * reintroduce Approving brand strings outside the allowlist.
 *
 * Allowed (plan constraints):
 * - CHANGELOG.md historical text
 * - github.com/cocofhu/approving* / approving-pages / approving-ai.com
 * - buttons.approving / i18n verb "Approving…"
 * - migrateBrandStorage LEGACY_* tables and approving-* as migration sources
 * - LEGACY_PRODUCT_NAME / BrandLogo stored Approving → display Grasp
 * - comments documenting the old onboarding-dismiss key
 * - node type=approve / Approve node concept
 */
import { execSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

const patterns = [
  String.raw`approving-theme`,
  String.raw`approving-locale`,
  String.raw`approving-sidebar-hidden`,
  String.raw`approving-drafts`,
  String.raw`approving-project-context`,
  String.raw`approving-runs-status-filter`,
  String.raw`approving-onboarding-suppress:`,
  String.raw`approving\.workflowFavorites\.`,
  String.raw`approving\.home\.`,
  String.raw`approving\.gateShareUrl\.`,
  String.raw`application/approving-node`,
  String.raw`/__approving/`,
  String.raw`X-Approving-Doctor-Cleanup`,
  String.raw`approving-sb-`,
  String.raw`approving\.managed`,
  String.raw`approving\.name`,
  String.raw`approving-vcs:`,
  String.raw`approving-mcp-spa-proxy`,
  String.raw`approving-mcp-advertise`,
  String.raw`/tmp/approving-ssh-inject`,
  String.raw`approving-acp-`,
]

const allowPathSubstrings = [
  'CHANGELOG.md',
  'migrateBrandStorage.ts',
  'migrateBrandStorage.test.ts',
  'useBrandSettings.ts',
  'BrandLogo.test.ts',
  'scripts/assert-no-approving-brand.mjs',
  'web/index.html', // boot script may read legacy once
  'docs/site/js/locale.js', // may read legacy once
  'theme.test.ts',
  'locale.test.ts',
  'useWorkflowFavorites.ts', // legacyFavoritesKeyForUser migration source
]

const allowLineRegexes = [
  /github\.com\/cocofhu\/approving/,
  /approving-pages/,
  /approving-ai\.com/,
  /buttons\.approving/,
  /LEGACY_PRODUCT_NAME/,
  /LEGACY_STORAGE_KEYS/,
  /LEGACY_DRAFT_IDB_NAME/,
  /LEGACY_KEY/,
  /approving-onboarding-dismiss:/,
  /"approving":\s*"Approving/,
  /product_name:\s*'Approving'/,
  /migrate from approving-/,
  /copy stores from approving-drafts/,
  /localStorage\.getItem\('approving-locale'\)/,
  /localStorage\.getItem\("approving-locale"\)/,
  /removeItem\('approving-locale'\)/,
  /removeItem\("approving-locale"\)/,
  /setItem\('approving-theme'/, // migration unit tests
  /setItem\('approving-locale'/,
  /setItem\("approving-locale"/,
  /approving\.workflowFavorites\.alice/, // migration unit tests
  /approving\.gateShareUrl\.r1/,
  /notifications\.prefs/,
  /runTerminalNotifications\.readIds/,
]

function main() {
  const pattern = patterns.join('|')
  let out = ''
  try {
    out = execSync(
      `rg -n --no-heading -g '!**/node_modules/**' -g '!**/.git/**' -g '!CHANGELOG.md' -e '${pattern}' .`,
      { cwd: root, encoding: 'utf8', maxBuffer: 20 * 1024 * 1024 },
    )
  } catch (e) {
    if (e.status === 1) {
      console.log('assert-no-approving-brand: ok')
      return
    }
    console.error(e.stderr || e.message)
    process.exit(2)
  }

  const offenders = []
  for (const line of out.split('\n')) {
    if (!line.trim()) continue
    const colon = line.indexOf(':')
    const file = colon === -1 ? line : line.slice(0, colon)
    if (allowPathSubstrings.some((s) => file.includes(s))) continue
    if (allowLineRegexes.some((re) => re.test(line))) continue
    offenders.push(line)
  }

  if (offenders.length) {
    console.error('assert-no-approving-brand: forbidden brand remnants:\n')
    for (const o of offenders.slice(0, 80)) console.error(o)
    if (offenders.length > 80) console.error(`… and ${offenders.length - 80} more`)
    process.exit(1)
  }
  console.log('assert-no-approving-brand: ok')
}

main()

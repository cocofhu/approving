/** Run-scoped sandbox env helpers (StartRun snapshot). */

export type RunSandboxEnvEntry = {
  key: string
  value: string
  secret?: boolean
}

const DENIED_EXACT = new Set([
  'CURSOR_API_KEY',
  'ANTHROPIC_API_KEY',
  'CODEBUDDY_API_KEY',
  'TRAE_API_KEY',
  'TRAECLI_PERSONAL_ACCESS_TOKEN',
  'GRASP_CURSOR_API_KEY',
  'GRASP_CLAUDE_API_KEY',
  'GRASP_CODEBUDDY_API_KEY',
  'GRASP_TRAE_API_KEY',
  'GRASP_OPENCODE_API_KEY',
  'OPENCODE_API_KEY',
  'PASSWORD',
  'ROOT_PASSWORD',
  'ACP_BRIDGE_PASSWORD',
  'CURSOR_ACP_PASSWORD',
  'GRASP_ARTIFACT_URL',
  'GRASP_ARTIFACT_TOKEN',
  'GRASP_RUN_ID',
  'GRASP_NODE_ID',
  'ACP_BACKEND',
  'CONFIG_ROOT',
  'SSH_KEY',
  'GIT_REPOS',
])

export function isDeniedRunSandboxEnvKey(key: string): boolean {
  const k = key.trim()
  if (!k) return false
  if (DENIED_EXACT.has(k)) return true
  return k.startsWith('GRASP_ARTIFACT_')
}

/** Collect effective rows (skip double-empty) and list validation problems. */
export function validateRunSandboxEnvRows(rows: RunSandboxEnvEntry[]): {
  entries: RunSandboxEnvEntry[]
  problems: string[]
} {
  const entries: RunSandboxEnvEntry[] = []
  const problems: string[] = []
  const seen = new Set<string>()
  rows.forEach((row, i) => {
    const key = row.key.trim()
    const value = row.value ?? ''
    if (!key && value === '') return
    if (!key) {
      problems.push(`row ${i + 1} (missing key)`)
      return
    }
    if (seen.has(key)) {
      problems.push(`${key} (duplicate)`)
      return
    }
    seen.add(key)
    if (isDeniedRunSandboxEnvKey(key)) {
      problems.push(key)
      return
    }
    entries.push({ key, value, secret: !!row.secret })
  })
  return { entries, problems }
}

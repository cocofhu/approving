/** COMPAT(approving→grasp): remove after next minor. */

export const LEGACY_ENV_PREFIX = 'APPROVING_'
export const ENV_PREFIX = 'GRASP_'

/** Rewrite an APPROVING_* name to GRASP_*; other names are unchanged. */
export function migrateLegacyEnvKey(key: string): string {
  const k = key.trim()
  if (k.startsWith(LEGACY_ENV_PREFIX)) return ENV_PREFIX + k.slice(LEGACY_ENV_PREFIX.length)
  return k
}

/** Rewrite APPROVING_ to GRASP_ inside interpolations and names. */
export function rewriteLegacyEnvText(value: string): string {
  return value.includes(LEGACY_ENV_PREFIX) ? value.split(LEGACY_ENV_PREFIX).join(ENV_PREFIX) : value
}

/** Rename APPROVING_* keys to GRASP_*. Existing GRASP_* values win. */
export function migrateLegacyEnvRecord(
  env: Record<string, string> | undefined | null,
): Record<string, string> {
  const src = env || {}
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(src)) {
    if (!k.startsWith(LEGACY_ENV_PREFIX)) out[k] = rewriteLegacyEnvText(v)
  }
  for (const [k, v] of Object.entries(src)) {
    if (!k.startsWith(LEGACY_ENV_PREFIX)) continue
    const dest = migrateLegacyEnvKey(k)
    if (Object.prototype.hasOwnProperty.call(out, dest)) continue
    out[dest] = rewriteLegacyEnvText(v)
  }
  return out
}

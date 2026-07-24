import type { Plugin } from 'vite'

/**
 * Strip noVNC's Secure Context warning so HTTP (non-localhost) deployments work.
 * x11vnc -nopw uses None auth; crypto.subtle is not required for the preview path.
 * Localhost is already a Secure Context — this only matters for http://hostname.
 */
export const SECURE_CONTEXT_SNIPPET = 'noVNC requires a secure context (TLS)'

const BLOCK_RE =
  /\/\/ We rely on modern APIs which might not be available in an\s*\n\s*\/\/ insecure context\s*\n\s*if\s*\(\s*!window\.isSecureContext\s*\)\s*\{\s*\n\s*Log\.Error\("noVNC requires a secure context \(TLS\)\. Expect crashes!"\);\s*\n\s*\}\s*\n?/

/** Pure transform used by the Vite plugin and unit tests. */
export function stripNovncSecureContextCheck(code: string): string | null {
  if (!code.includes(SECURE_CONTEXT_SNIPPET)) return null
  const next = code.replace(BLOCK_RE, '')
  return next === code ? null : next
}

export function stripNovncSecureContext(): Plugin {
  return {
    name: 'strip-novnc-secure-context',
    transform(code, id) {
      // Match both prebundled and source paths for @novnc/novnc/.../rfb.js
      if (!id.includes('@novnc/novnc') || !id.includes('rfb.js')) return null
      const next = stripNovncSecureContextCheck(code)
      if (next == null) return null
      return { code: next, map: null }
    },
  }
}

import type { BackendId } from '@/lib/shared/regionPolicy'

export const OPENCODE_PROVIDER_ENV = 'APPROVING_OPENCODE_PROVIDER'
export const OPENCODE_BASE_URL_ENV = 'APPROVING_OPENCODE_BASE_URL'
export const OPENCODE_MODEL_ENV = 'ACP_BRIDGE_MODEL'
export const DEFAULT_OPENCODE_PROVIDER = 'openai'

/**
 * A vendor id from the OpenCode catalog (models.dev), or `custom` for a gateway
 * absent from it. The catalog holds hundreds and grows without us, so ids are not
 * enumerated here — the picker reads them from the server.
 */
export type OpenCodeProviderId = string

/** The id `custom` is ours, not the catalog's: it carries a base URL and model. */
export const OPENCODE_CUSTOM_PROVIDER = 'custom'

/**
 * Vendors offered when the catalog cannot be reached, so an offline install still
 * has a usable list. Labels are translated; models are never listed here, because
 * a hand-written model list rots into ids OpenCode rejects.
 */
export const OPENCODE_FALLBACK_PROVIDERS: { id: OpenCodeProviderId; labelKey: string }[] = [
  { id: 'openai', labelKey: 'pages.agentStudio.openCode.providers.openai' },
  { id: 'anthropic', labelKey: 'pages.agentStudio.openCode.providers.anthropic' },
  { id: 'google', labelKey: 'pages.agentStudio.openCode.providers.google' },
  { id: 'openrouter', labelKey: 'pages.agentStudio.openCode.providers.openrouter' },
  { id: 'deepseek', labelKey: 'pages.agentStudio.openCode.providers.deepseek' },
  { id: 'moonshotai', labelKey: 'pages.agentStudio.openCode.providers.moonshotai' },
  { id: 'alibaba', labelKey: 'pages.agentStudio.openCode.providers.alibaba' },
  { id: 'xai', labelKey: 'pages.agentStudio.openCode.providers.xai' },
  { id: OPENCODE_CUSTOM_PROVIDER, labelKey: 'pages.agentStudio.openCode.providers.custom' },
]

/** Mirrors the server: a catalog-shaped id survives, anything else is custom. */
const PROVIDER_ID_SHAPE = /^[a-z0-9][a-z0-9._-]*$/

export function normalizeOpenCodeProvider(raw: string): OpenCodeProviderId {
  const v = raw.trim().toLowerCase()
  if (!v) return DEFAULT_OPENCODE_PROVIDER
  if (!PROVIDER_ID_SHAPE.test(v)) return OPENCODE_CUSTOM_PROVIDER
  return v
}

/**
 * Whether the model belongs to the vendor. OpenCode takes `provider/model`, so a
 * model kept across a vendor switch would name the wrong vendor.
 */
export function openCodeModelMatchesProvider(model: string, provider: string): boolean {
  const m = model.trim()
  if (!m) return true
  // A bare id names no vendor to contradict; the server prefixes it on write.
  if (!m.includes('/')) return true
  return m.startsWith(`${normalizeOpenCodeProvider(provider)}/`)
}

export type OpenCodeFields = {
  provider: OpenCodeProviderId
  baseURL: string
  model: string
}

export function openCodeFieldsFromEnv(env: Record<string, string>): OpenCodeFields {
  return {
    provider: normalizeOpenCodeProvider(env[OPENCODE_PROVIDER_ENV] || ''),
    baseURL: (env[OPENCODE_BASE_URL_ENV] || '').trim(),
    model: (env[OPENCODE_MODEL_ENV] || '').trim(),
  }
}

export function applyOpenCodeFields(
  env: Record<string, string>,
  fields: Partial<OpenCodeFields>,
): Record<string, string> {
  const current = openCodeFieldsFromEnv(env)
  const next: OpenCodeFields = {
    provider: fields.provider ?? current.provider,
    baseURL: fields.baseURL !== undefined ? fields.baseURL.trim() : current.baseURL,
    model: fields.model !== undefined ? fields.model.trim() : current.model,
  }
  const out = { ...env }
  out[OPENCODE_PROVIDER_ENV] = next.provider
  if (next.baseURL) out[OPENCODE_BASE_URL_ENV] = next.baseURL
  else delete out[OPENCODE_BASE_URL_ENV]
  if (next.model) out[OPENCODE_MODEL_ENV] = next.model
  else delete out[OPENCODE_MODEL_ENV]
  return out
}

export function switchOpenCodeEnv(
  env: Record<string, string>,
  backend: BackendId,
): Record<string, string> {
  const out = { ...env }
  delete out[OPENCODE_PROVIDER_ENV]
  delete out[OPENCODE_BASE_URL_ENV]
  if (backend === 'opencode') {
    out[OPENCODE_PROVIDER_ENV] = DEFAULT_OPENCODE_PROVIDER
  }
  return out
}

export function openCodeCustomBaseRequired(provider: string, baseURL: string): boolean {
  return normalizeOpenCodeProvider(provider) === 'custom' && !baseURL.trim()
}

export function openCodeModelRequired(model: string): boolean {
  return !model.trim()
}

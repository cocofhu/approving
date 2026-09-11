import { api } from '@/lib/api/api'
import type { OpenCodeCatalogModel, OpenCodeCatalogProvider } from '@/lib/api/apiTypes'

/**
 * Reads the OpenCode model catalog through the server.
 *
 * The catalog is a convenience, never a gate: every failure resolves to an empty
 * list, and the picker falls back to a hand-typed id, which OpenCode accepts just
 * the same. Answers are memoized for the lifetime of the page, since the vendor
 * list does not change while a form is open.
 */

let providersOnce: Promise<OpenCodeCatalogProvider[]> | null = null
let providerIds: Set<string> | null = null
const modelsOnce = new Map<string, Promise<OpenCodeCatalogModel[]>>()

export function loadOpenCodeProviders(): Promise<OpenCodeCatalogProvider[]> {
  if (!providersOnce) {
    providersOnce = api
      .openCodeProviders()
      .then((r) => {
        if (r?.error) {
          providerIds = null
          return []
        }
        const providers = r?.providers || []
        providerIds = new Set(providers.map((p) => p.id.trim().toLowerCase()))
        return providers
      })
      .catch(() => {
        providerIds = null
        return []
      })
  }
  return providersOnce
}

/**
 * Reports whether the last readable catalog lists a provider. Undefined means
 * it has not loaded or could not be read, so callers must stay conservative.
 */
export function openCodeCatalogKnowsProvider(provider: string): boolean | undefined {
  if (!providerIds) return undefined
  return providerIds.has(provider.trim().toLowerCase())
}

export function loadOpenCodeModels(provider: string): Promise<OpenCodeCatalogModel[]> {
  const id = provider.trim().toLowerCase()
  if (!id) return Promise.resolve([])
  const cached = modelsOnce.get(id)
  if (cached) return cached
  const pending = api
    .openCodeModels(id)
    .then((r) => r?.models || [])
    .catch(() => [])
  modelsOnce.set(id, pending)
  return pending
}

/** Drops the memoized answers; tests and a re-login want a clean slate. */
export function resetOpenCodeCatalog(): void {
  providersOnce = null
  providerIds = null
  modelsOnce.clear()
}

import { ACP_BACKENDS, type BackendId } from '@/lib/shared/regionPolicy'

/**
 * Two ways to start an Agent: bring a model-vendor API key (BYOK, served by
 * OpenCode), or sign in with a coding-CLI vendor account (Cursor / Claude Code /
 * CodeBuddy / Trae). Shared by onboarding, create-Agent, and team wizards.
 */
export type StartPath = 'apiKey' | 'cli'

/** BYOK path is served by a single backend; keep the mapping in one place. */
export const APIKEY_BACKEND: BackendId = 'opencode'

export const CLI_BACKEND_DEFAULT: BackendId = 'cursor'

/** CLI-account backends, in ACP_BACKENDS order (BYOK backend excluded). */
export const CLI_BACKENDS = ACP_BACKENDS.filter((b) => b.id !== APIKEY_BACKEND)

export function startPathForBackend(backend: BackendId): StartPath {
  return backend === APIKEY_BACKEND ? 'apiKey' : 'cli'
}

/** Resolve the Backend that a path switch should land on. */
export function backendForStartPath(path: StartPath, cliBackend: BackendId): BackendId {
  return path === 'apiKey' ? APIKEY_BACKEND : cliBackend || CLI_BACKEND_DEFAULT
}

/** Path-card i18n; shared so create/team wizards match onboarding copy. */
export const START_PATH_OPTIONS: {
  id: StartPath
  titleKey: string
  descKey: string
}[] = [
  {
    id: 'apiKey',
    titleKey: 'pages.onboarding.acp.paths.apiKey.title',
    descKey: 'pages.onboarding.acp.paths.apiKey.desc',
  },
  {
    id: 'cli',
    titleKey: 'pages.onboarding.acp.paths.cli.title',
    descKey: 'pages.onboarding.acp.paths.cli.desc',
  },
]

/** Minimal draft shape for path ↔ Backend sync (onboarding / wizard drafts). */
export type StartPathDraft = {
  startPath: StartPath
  acpBackend: BackendId
  cliBackend: BackendId
}

/**
 * Keep startPath / cliBackend aligned with the selected Backend.
 * Does not touch configRoot / env — callers apply those separately.
 */
export function syncStartPathFields(draft: StartPathDraft, id: BackendId): void {
  const path = startPathForBackend(id)
  draft.startPath = path
  if (path === 'cli') draft.cliBackend = id
}

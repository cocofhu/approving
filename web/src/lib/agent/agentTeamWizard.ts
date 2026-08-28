import type { MCPServer } from '@/lib/api/api'
import {
  ACP_BACKENDS,
  applyAcpBackend,
  configRootFor,
  kvToRec,
  normalizeWizardRegions,
  recToKV,
  type WizardBackendId,
  type WizardDraft,
  type WizardKV,
  type WizardMCP,
  type WizardAuthMode,
  freshDraft,
  parseCustomConfigJson,
  stripAuthKeysFromEnv,
} from '@/lib/agent/agentCreateWizard'
import { normalizeAgentName, validateAgentName } from '@/lib/agent/agentIO'
import type { GitCredentialType } from '@/lib/agent/gitCredentialAnalysis'
import { authGuideFor, hasAuthKeyConfigured } from '@/lib/agent/backendAuthGuide'
import { stripTokenKeysFromKV, stripTokenKeysFromRecord } from '@/lib/agent/tokenEnvKeys'
import { getRegionPolicy } from '@/lib/shared/regionPolicy'

export type TeamWizardStepId = 'team' | 'acp' | 'apiKey' | 'git' | 'mcp' | 'env' | 'review'

export type TeamWizardStepDef = {
  id: TeamWizardStepId
  labelKey: string
  skip: boolean
}

export const TEAM_WIZARD_STEPS: TeamWizardStepDef[] = [
  { id: 'team', labelKey: 'pages.agentStudio.teamWizard.steps.team', skip: false },
  { id: 'acp', labelKey: 'pages.agentStudio.teamWizard.steps.acp', skip: true },
  { id: 'apiKey', labelKey: 'pages.agentStudio.teamWizard.steps.apiKey', skip: true },
  { id: 'git', labelKey: 'pages.agentStudio.teamWizard.steps.git', skip: true },
  { id: 'mcp', labelKey: 'pages.agentStudio.teamWizard.steps.mcp', skip: true },
  { id: 'env', labelKey: 'pages.agentStudio.teamWizard.steps.env', skip: true },
  { id: 'review', labelKey: 'pages.agentStudio.teamWizard.steps.review', skip: false },
]

export const TEAM_ENGINEER_COUNT = 9

export type TeamWizardDraft = {
  step: number
  projectName: string
  prefix: string
  rootGroupName: string
  pipelineGroupName: string
  pmName: string
  background: string
  prefixTouched: boolean
  rootTouched: boolean
  pipelineTouched: boolean
  pmTouched: boolean
  acpBackend: WizardBackendId
  authMode: WizardAuthMode
  customConfigContent: string
  gitCredentialType?: GitCredentialType
  gitUrl: string
  configRoot: string
  env: WizardKV[]
  mcp: WizardMCP[]
  skipped: Partial<Record<TeamWizardStepId, boolean>>
}

export function artifactStorePreset(): WizardMCP {
  return {
    name: 'artifact-store',
    transport: 'url',
    url: '${APPROVING_ARTIFACT_URL}',
    headers: [{ k: 'Authorization', v: 'Bearer ${APPROVING_ARTIFACT_TOKEN}' }],
    command: '',
    args: '',
    env: [],
  }
}

export function freshTeamDraft(): TeamWizardDraft {
  return {
    step: 0,
    projectName: '',
    prefix: '',
    rootGroupName: '',
    pipelineGroupName: 'Pipeline(GitHub)',
    pmName: '',
    background: '',
    prefixTouched: false,
    rootTouched: false,
    pipelineTouched: false,
    pmTouched: false,
    acpBackend: 'cursor',
    authMode: 'apiKey',
    customConfigContent: '',
    gitCredentialType: undefined,
    gitUrl: '',
    configRoot: configRootFor('cursor'),
    env: [{ k: 'GIT_REPOS', v: '${vars.repos}' }],
    mcp: [artifactStorePreset()],
    skipped: {},
  }
}

export function syncDerivedNames(d: TeamWizardDraft) {
  const base = (d.prefixTouched ? d.prefix : d.projectName).trim() || d.projectName.trim()
  if (!d.prefixTouched) d.prefix = d.projectName.trim()
  if (!d.rootTouched) d.rootGroupName = base ? `${base}项目组` : ''
  if (!d.pmTouched) d.pmName = base ? `${base}项目经理` : ''
}

export function validateTeamBasics(d: TeamWizardDraft, existingNames: string[]): string {
  if (!d.projectName.trim()) return 'projectRequired'
  if (!d.prefix.trim()) return 'prefixRequired'
  if (!d.background.trim()) return 'backgroundRequired'
  const code = validateAgentName(d.pmName)
  if (code === 'required') return 'pmRequired'
  if (code === 'invalid') return 'pmInvalid'
  const normalized = normalizeAgentName(d.pmName)
  if (existingNames.some((n) => normalizeAgentName(n) === normalized)) return 'pmExists'
  return ''
}

function teamDraftAsWizardDraft(d: TeamWizardDraft): WizardDraft {
  const base = freshDraft()
  base.acpBackend = d.acpBackend
  base.configRoot = d.configRoot
  base.authMode = d.authMode
  base.customConfigContent = d.customConfigContent
  base.env = d.env.map((e) => ({ ...e }))
  base.gitCredentialType = d.gitCredentialType
  base.mcp = d.mcp.map((m) => ({
    ...m,
    headers: m.headers.map((h) => ({ ...h })),
    env: m.env.map((e) => ({ ...e })),
  }))
  return base
}

export function applyTeamAcpBackend(d: TeamWizardDraft, id: WizardBackendId) {
  const w = teamDraftAsWizardDraft(d)
  applyAcpBackend(w, id)
  d.acpBackend = w.acpBackend
  d.configRoot = w.configRoot
  d.env = w.env
}

function mcpToApi(m: WizardMCP): MCPServer {
  if (m.transport === 'command') {
    return {
      name: m.name.trim(),
      command: m.command.trim(),
      args: m.args
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
      env: kvToRec(m.env),
    }
  }
  return {
    name: m.name.trim(),
    url: m.url.trim(),
    headers: kvToRec(m.headers),
  }
}

export type TeamBootstrapPayload = {
  projectName: string
  prefix: string
  rootGroupName: string
  pipelineGroupName: string
  pmName: string
  background: string
  acpBackend: string
  apiKey?: string
  customConfig?: string
  region?: string
  gitUrl?: string
  gitCredentialType?: string
  mcp: MCPServer[]
  env: Record<string, string>
}

export function assembleTeamBootstrapPayload(d: TeamWizardDraft): TeamBootstrapPayload {
  const w = teamDraftAsWizardDraft(d)
  const policy = getRegionPolicy(d.acpBackend)
  const region = policy
    ? w.env.find((e) => e.k.trim() === policy.regionEnvKey)?.v?.trim() || undefined
    : undefined
  const guide = authGuideFor(d.acpBackend, region || '')
  const primaryKey = guide.keys[0]?.key || ''
  const customParsed = parseCustomConfigJson(d.customConfigContent)
  const customConfig =
    d.authMode === 'customConfig' && customParsed.ok && customParsed.normalized
      ? customParsed.normalized
      : undefined
  // Capture API Key for project/shared layer before stripping Token keys from agent env.
  const rawEnv = kvToRec(w.env)
  const apiKey =
    !customConfig && primaryKey ? rawEnv[primaryKey]?.trim() || undefined : undefined

  if (w.authMode === 'customConfig') {
    w.env = stripAuthKeysFromEnv(w.env, w.acpBackend)
  }
  w.env = stripTokenKeysFromKV(w.env)
  const env = stripTokenKeysFromRecord(normalizeWizardRegions(w))
  d.env = w.env
  return {
    projectName: d.projectName.trim(),
    prefix: d.prefix.trim(),
    rootGroupName: (d.rootGroupName.trim() || `${d.prefix.trim()}项目组`),
    pipelineGroupName: d.pipelineGroupName.trim() || 'Pipeline(GitHub)',
    pmName: normalizeAgentName(d.pmName),
    background: d.background.trim(),
    acpBackend: d.acpBackend || 'cursor',
    ...(apiKey ? { apiKey } : {}),
    ...(customConfig ? { customConfig } : {}),
    ...(region ? { region } : {}),
    ...(d.gitUrl.trim() ? { gitUrl: d.gitUrl.trim() } : {}),
    ...(d.gitCredentialType ? { gitCredentialType: d.gitCredentialType } : {}),
    mcp: d.mcp.filter((m) => m.name.trim()).map(mcpToApi),
    env,
  }
}

export function teamHasAuth(d: TeamWizardDraft): boolean {
  if (d.authMode === 'customConfig') {
    const parsed = parseCustomConfigJson(d.customConfigContent)
    return parsed.ok && parsed.normalized !== ''
  }
  return hasAuthKeyConfigured(d.env, d.acpBackend)
}

export { ACP_BACKENDS, recToKV, kvToRec }
export type { WizardBackendId }
